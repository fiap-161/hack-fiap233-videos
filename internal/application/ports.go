package application

import (
	"context"
	"io"

	"github.com/hack-fiap233/videos/internal/domain"
)

// Port de persistência (adapter driven)
type VideoRepository interface {
	ListByUserID(ctx context.Context, userID int) ([]domain.Video, error)
	Create(ctx context.Context, userID int, title, description string) (domain.Video, error)
	CreateWithStorageKey(ctx context.Context, userID int, title, description, storageKey string) (domain.Video, error)
	GetByID(ctx context.Context, videoID int) (domain.Video, error)
	SetProcessing(ctx context.Context, videoID int) error
	SetCompleted(ctx context.Context, videoID int, resultZipPath string) error
	SetFailed(ctx context.Context, videoID int, errorMessage string) error
}

type HealthChecker interface {
	Ping(ctx context.Context) error
}

// Storage guarda e recupera objetos (vídeo original, ZIP de resultado). Em prod: S3/MinIO.
type Storage interface {
	Upload(ctx context.Context, key string, body io.Reader, contentType string) error
	Download(ctx context.Context, key string) (io.ReadCloser, error)
}

// VideoQueue publica jobs de processamento (ex: RabbitMQ video.process).
type VideoQueue interface {
	PublishVideoJob(ctx context.Context, videoID, userID int, userEmail, storageKey string) error
}

// FailureNotifier publica evento de falha para notificação (ex.: SNS video-processing-failed).
type FailureNotifier interface {
	NotifyProcessingFailed(ctx context.Context, userID int, userEmail string, videoID int, errorMessage string) error
}

// VideoProcessor gera um ZIP a partir de um vídeo (ex.: extração de frames). Input/output são paths locais.
type VideoProcessor interface {
	Process(ctx context.Context, videoLocalPath string) (zipLocalPath string, err error)
}
