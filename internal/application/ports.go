package application

import (
	"context"

	"github.com/hack-fiap233/videos/internal/domain"
)

// Port de persistência (adapter driven)
type VideoRepository interface {
	ListByUserID(ctx context.Context, userID int) ([]domain.Video, error)
	Create(ctx context.Context, userID int, title, description string) (domain.Video, error)
}

type HealthChecker interface {
	Ping(ctx context.Context) error
}
