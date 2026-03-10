package application

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"time"

	"github.com/hack-fiap233/videos/internal/domain"
)

var (
	ErrTitleRequired   = errors.New("title is required")
	ErrVideoNotFound  = errors.New("video not found")
	ErrInvalidStatus  = errors.New("video not in pending status")
	ErrStorageKeyMissing = errors.New("video has no storage key")
)

// VideoService implementa os use cases da aplicação
type VideoService struct {
	repo     VideoRepository
	storage  Storage
	queue    VideoQueue
	notifier FailureNotifier
}

// VideoServiceOption permite injetar dependências opcionais (queue, notifier) para o worker.
type VideoServiceOption func(*VideoService)

func NewVideoService(repo VideoRepository, storage Storage, opts ...VideoServiceOption) *VideoService {
	s := &VideoService{repo: repo, storage: storage}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func WithQueue(q VideoQueue) VideoServiceOption   { return func(s *VideoService) { s.queue = q } }
func WithNotifier(n FailureNotifier) VideoServiceOption { return func(s *VideoService) { s.notifier = n } }

func (s *VideoService) ListByUser(ctx context.Context, userID int) ([]domain.Video, error) {
	return s.repo.ListByUserID(ctx, userID)
}

// GetByIDForUser retorna o vídeo só se pertencer ao userID; else ErrVideoNotFound.
func (s *VideoService) GetByIDForUser(ctx context.Context, userID, videoID int) (domain.Video, error) {
	v, err := s.repo.GetByID(ctx, videoID)
	if err != nil {
		return domain.Video{}, ErrVideoNotFound
	}
	if v.UserID != userID {
		return domain.Video{}, ErrVideoNotFound
	}
	return v, nil
}

// DownloadResultZip retorna o stream do ZIP e um nome sugerido para download (ex: video_123.zip).
// Só permite se o vídeo for do usuário e estiver completed
func (s *VideoService) DownloadResultZip(ctx context.Context, userID, videoID int) (io.ReadCloser, string, error) {
	v, err := s.GetByIDForUser(ctx, userID, videoID)
	if err != nil {
		return nil, "", err
	}
	if v.Status != domain.StatusCompleted {
		return nil, "", ErrInvalidStatus
	}
	if v.ResultZipPath == "" {
		return nil, "", ErrStorageKeyMissing
	}
	reader, err := s.storage.Download(ctx, v.ResultZipPath)
	if err != nil {
		return nil, "", fmt.Errorf("storage download: %w", err)
	}
	filename := fmt.Sprintf("video_%d.zip", videoID)
	return reader, filename, nil
}

// CreateVideo cria um vídeo com status pending (só metadado; upload usa UploadVideo).
func (s *VideoService) CreateVideo(ctx context.Context, userID int, title, description string) (domain.Video, error) {
	if title == "" {
		return domain.Video{}, ErrTitleRequired
	}
	return s.repo.Create(ctx, userID, title, description)
}

// UploadVideo salva o arquivo no storage, cria o registro com status pending e publica job na fila
func (s *VideoService) UploadVideo(ctx context.Context, userID int, userEmail, title, description string, file io.Reader, contentType string) (domain.Video, error) {
	if title == "" {
		title = "video"
	}
	storageKey := fmt.Sprintf("videos/%d/%d.%s", userID, time.Now().UnixNano(), extensionFromContentType(contentType))
	if err := s.storage.Upload(ctx, storageKey, file, contentType); err != nil {
		return domain.Video{}, fmt.Errorf("storage upload: %w", err)
	}
	v, err := s.repo.CreateWithStorageKey(ctx, userID, title, description, storageKey)
	if err != nil {
		return domain.Video{}, err
	}
	if s.queue != nil {
		if err := s.queue.PublishVideoJob(ctx, v.ID, userID, userEmail, storageKey); err != nil {
			// Não falhamos o upload; o job pode ser reenfileirado ou reprocessado manualmente
			return v, nil
		}
	}
	return v, nil
}

func extensionFromContentType(ct string) string {
	switch {
	case ct == "video/mp4" || len(ct) > 4 && ct[:4] == "video":
		return "mp4"
	default:
		return "bin"
	}
}

// ProcessJob processa um job da fila: baixa vídeo, processa (frames → ZIP), sobe ZIP, atualiza status; em falha notifica.
// userEmail vem do payload da fila e é usado na notificação de falha (SNS).
func (s *VideoService) ProcessJob(ctx context.Context, videoID int, userEmail string, processor interface {
	Process(ctx context.Context, videoLocalPath string) (zipLocalPath string, err error)
}) error {
	v, err := s.repo.GetByID(ctx, videoID)
	if err != nil {
		return fmt.Errorf("get video: %w", err)
	}
	if v.Status != domain.StatusPending {
		return ErrInvalidStatus // idempotência: já processado ou em processamento
	}
	if v.StorageKey == "" {
		return ErrStorageKeyMissing
	}
	if err := s.repo.SetProcessing(ctx, videoID); err != nil {
		return err
	}
	reader, err := s.storage.Download(ctx, v.StorageKey)
	if err != nil {
		s.failAndNotify(ctx, videoID, v.UserID, userEmail, fmt.Sprintf("download: %v", err))
		return err
	}
	tmpFile, err := os.CreateTemp("", "video-*.tmp")
	if err != nil {
		reader.Close()
		s.failAndNotify(ctx, videoID, v.UserID, userEmail, fmt.Sprintf("temp file: %v", err))
		return err
	}
	_, _ = io.Copy(tmpFile, reader)
	reader.Close()
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	zipPath, err := processor.Process(ctx, tmpPath)
	if err != nil {
		s.failAndNotify(ctx, videoID, v.UserID, userEmail, fmt.Sprintf("process: %v", err))
		return err
	}
	zipKey := resultZipKey(v.StorageKey)
	fz, err := os.Open(zipPath)
	if err != nil {
		s.failAndNotify(ctx, videoID, v.UserID, userEmail, fmt.Sprintf("open zip: %v", err))
		return err
	}
	defer fz.Close()
	if err := s.storage.Upload(ctx, zipKey, fz, "application/zip"); err != nil {
		s.failAndNotify(ctx, videoID, v.UserID, userEmail, fmt.Sprintf("upload zip: %v", err))
		return err
	}
	if err := s.repo.SetCompleted(ctx, videoID, zipKey); err != nil {
		return err
	}
	return nil
}

func (s *VideoService) failAndNotify(ctx context.Context, videoID, userID int, userEmail, errMsg string) {
	_ = s.repo.SetFailed(ctx, videoID, errMsg)
	if s.notifier != nil {
		_ = s.notifier.NotifyProcessingFailed(ctx, userID, userEmail, videoID, errMsg)
	}
}

func resultZipKey(storageKey string) string {
	dir, base := path.Split(storageKey)
	ext := path.Ext(base)
	name := base[:len(base)-len(ext)]
	return path.Join(dir, name+".zip")
}
