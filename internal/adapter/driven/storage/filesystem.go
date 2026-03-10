package storage

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/hack-fiap233/videos/internal/application"
)

// FilesystemStorage implementa Storage usando o disco local (dev/minio-style paths).
// BasePath é o diretório raiz onde os objetos são guardados (ex: ./data ou /tmp/videos-storage).
type FilesystemStorage struct {
	BasePath string
}

// Garante que FilesystemStorage implementa application.Storage.
var _ application.Storage = (*FilesystemStorage)(nil)

func NewFilesystemStorage(basePath string) (*FilesystemStorage, error) {
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, err
	}
	return &FilesystemStorage{BasePath: basePath}, nil
}

func (s *FilesystemStorage) Upload(ctx context.Context, key string, body io.Reader, _ string) error {
	fullPath := filepath.Join(s.BasePath, key)
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.Create(fullPath)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, body)
	return err
}

func (s *FilesystemStorage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	fullPath := filepath.Join(s.BasePath, key)
	return os.Open(fullPath)
}
