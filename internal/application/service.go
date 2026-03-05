package application

import (
	"context"
	"errors"

	"github.com/hack-fiap233/videos/internal/domain"
)

var (
	ErrTitleRequired = errors.New("title is required")
)

// VideoService implementa os use cases da aplicação
type VideoService struct {
	repo VideoRepository
}

func NewVideoService(repo VideoRepository) *VideoService {
	return &VideoService{repo: repo}
}

func (s *VideoService) ListByUser(ctx context.Context, userID int) ([]domain.Video, error) {
	return s.repo.ListByUserID(ctx, userID)
}

// CreateVideo cria um vídeo com status pending
func (s *VideoService) CreateVideo(ctx context.Context, userID int, title, description string) (domain.Video, error) {
	if title == "" {
		return domain.Video{}, ErrTitleRequired
	}
	return s.repo.Create(ctx, userID, title, description)
}
