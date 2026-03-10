package processor

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/hack-fiap233/videos/internal/application"
)

var _ application.VideoProcessor = (*FFmpegProcessor)(nil)

// FFmpegProcessor extrai frames do vídeo com ffmpeg e gera um ZIP. Se ffmpeg não estiver disponível, gera ZIP placeholder.
type FFmpegProcessor struct {
	FramesDir string // diretório base para frames temporários (ex: os.TempDir())
}

func NewFFmpegProcessor(framesDir string) *FFmpegProcessor {
	if framesDir == "" {
		framesDir = os.TempDir()
	}
	return &FFmpegProcessor{FramesDir: framesDir}
}

// Process lê o vídeo em videoLocalPath, extrai frames (ou gera placeholder) e retorna o path do ZIP.
func (p *FFmpegProcessor) Process(ctx context.Context, videoLocalPath string) (zipLocalPath string, err error) {
	workDir, err := os.MkdirTemp(p.FramesDir, "video-process-*")
	if err != nil {
		return "", fmt.Errorf("mkdir temp: %w", err)
	}
	defer os.RemoveAll(workDir)

	framesDir := filepath.Join(workDir, "frames")
	if err := os.MkdirAll(framesDir, 0755); err != nil {
		return "", fmt.Errorf("mkdir frames: %w", err)
	}

	// Tenta extrair 1 frame a cada ~1s com ffmpeg; se não tiver ffmpeg, cria placeholder
	if err := p.extractFrames(ctx, videoLocalPath, framesDir); err != nil {
		// Fallback: criar um arquivo placeholder para o ZIP não ficar vazio
		_ = os.WriteFile(filepath.Join(framesDir, "readme.txt"), []byte("Frames could not be extracted (ffmpeg not available or error). Video was uploaded successfully.\n"), 0644)
	}

	zipPath := filepath.Join(workDir, "result.zip")
	if err := p.zipDir(framesDir, zipPath); err != nil {
		return "", fmt.Errorf("zip dir: %w", err)
	}
	return zipPath, nil
}

func (p *FFmpegProcessor) extractFrames(ctx context.Context, videoPath, outDir string) error {
	// 1 frame a cada 30 frames (~1 fps para 30fps) para não explodir o disco
	pattern := filepath.Join(outDir, "frame_%04d.jpg")
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-i", videoPath,
		"-vf", "fps=1/1", // 1 frame por segundo
		"-q:v", "3",
		"-y",
		pattern,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

func (p *FFmpegProcessor) zipDir(srcDir, zipPath string) error {
	f, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	defer f.Close()
	w := zip.NewWriter(f)
	defer w.Close()

	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(srcDir, path)
		rel = "frames/" + filepath.ToSlash(rel)
		header, _ := zip.FileInfoHeader(info)
		header.Name = rel
		header.Method = zip.Deflate
		entry, err := w.CreateHeader(header)
		if err != nil {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(entry, file)
		return err
	})
}

// EnsureZipExtension garante que a chave do storage termina em .zip (para resultado).
func EnsureZipExtension(key string) string {
	if strings.HasSuffix(key, ".zip") {
		return key
	}
	return key + ".zip"
}
