package postgres

import (
	"context"

	"github.com/hack-fiap233/videos/internal/application"
)

// Ensure VideoRepository implements application.HealthChecker.
var _ application.HealthChecker = (*VideoRepository)(nil)

// Ping implementa application.HealthChecker usando a conexão Postgres.
func (r *VideoRepository) Ping(ctx context.Context) error {
	return r.db.PingContext(ctx)
}
