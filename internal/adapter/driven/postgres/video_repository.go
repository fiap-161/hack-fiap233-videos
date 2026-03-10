package postgres

import (
	"context"
	"database/sql"

	"github.com/hack-fiap233/videos/internal/application"
	"github.com/hack-fiap233/videos/internal/domain"
)

// VideoRepository implementa o port application.VideoRepository usando Postgres.
type VideoRepository struct {
	db *sql.DB
}

// NewVideoRepository cria o repositório com a conexão injetada.
func NewVideoRepository(db *sql.DB) *VideoRepository {
	return &VideoRepository{db: db}
}

// EnsureVideoRepository implements application.VideoRepository.
var _ application.VideoRepository = (*VideoRepository)(nil)

// ListByUserID retorna os vídeos do usuário ordenados por id.
func (r *VideoRepository) ListByUserID(ctx context.Context, userID int) ([]domain.Video, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, title, description, status,
		 COALESCE(storage_key,''), COALESCE(result_zip_path,''), COALESCE(error_message,''),
		 created_at, updated_at
		 FROM videos WHERE user_id = $1 ORDER BY id`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []domain.Video
	for rows.Next() {
		var v domain.Video
		var createdAt, updatedAt sql.NullTime
		if err := rows.Scan(&v.ID, &v.UserID, &v.Title, &v.Description, &v.Status,
			&v.StorageKey, &v.ResultZipPath, &v.ErrorMessage, &createdAt, &updatedAt); err != nil {
			continue
		}
		if createdAt.Valid {
			v.CreatedAt = createdAt.Time.Format("2006-01-02T15:04:05Z07:00")
		}
		if updatedAt.Valid {
			v.UpdatedAt = updatedAt.Time.Format("2006-01-02T15:04:05Z07:00")
		}
		list = append(list, v)
	}
	if list == nil {
		list = []domain.Video{}
	}
	return list, nil
}

// Create insere um vídeo com status pending e retorna o registro criado.
func (r *VideoRepository) Create(ctx context.Context, userID int, title, description string) (domain.Video, error) {
	var v domain.Video
	var createdAt, updatedAt sql.NullTime
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO videos (user_id, title, description, status)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, user_id, title, description, status, created_at, updated_at`,
		userID, title, description, domain.StatusPending,
	).Scan(&v.ID, &v.UserID, &v.Title, &v.Description, &v.Status, &createdAt, &updatedAt)
	if err != nil {
		return domain.Video{}, err
	}
	if createdAt.Valid {
		v.CreatedAt = createdAt.Time.Format("2006-01-02T15:04:05Z07:00")
	}
	if updatedAt.Valid {
		v.UpdatedAt = updatedAt.Time.Format("2006-01-02T15:04:05Z07:00")
	}
	v.Status = domain.StatusPending
	return v, nil
}

// CreateWithStorageKey insere vídeo com status pending e chave do objeto no storage (upload já feito).
func (r *VideoRepository) CreateWithStorageKey(ctx context.Context, userID int, title, description, storageKey string) (domain.Video, error) {
	var v domain.Video
	var createdAt, updatedAt sql.NullTime
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO videos (user_id, title, description, status, storage_key)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, user_id, title, description, status,
		 COALESCE(storage_key,''), COALESCE(result_zip_path,''), COALESCE(error_message,''),
		 created_at, updated_at`,
		userID, title, description, domain.StatusPending, storageKey,
	).Scan(&v.ID, &v.UserID, &v.Title, &v.Description, &v.Status,
		&v.StorageKey, &v.ResultZipPath, &v.ErrorMessage, &createdAt, &updatedAt)
	if err != nil {
		return domain.Video{}, err
	}
	if createdAt.Valid {
		v.CreatedAt = createdAt.Time.Format("2006-01-02T15:04:05Z07:00")
	}
	if updatedAt.Valid {
		v.UpdatedAt = updatedAt.Time.Format("2006-01-02T15:04:05Z07:00")
	}
	return v, nil
}

// Retorna vídeo por ID ou erro se ele não existir.
func (r *VideoRepository) GetByID(ctx context.Context, videoID int) (domain.Video, error) {
	var v domain.Video
	var createdAt, updatedAt sql.NullTime
	err := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, title, description, status,
		 COALESCE(storage_key,''), COALESCE(result_zip_path,''), COALESCE(error_message,''),
		 created_at, updated_at
		 FROM videos WHERE id = $1`,
		videoID,
	).Scan(&v.ID, &v.UserID, &v.Title, &v.Description, &v.Status,
		&v.StorageKey, &v.ResultZipPath, &v.ErrorMessage, &createdAt, &updatedAt)
	if err != nil {
		return domain.Video{}, err
	}
	if createdAt.Valid {
		v.CreatedAt = createdAt.Time.Format("2006-01-02T15:04:05Z07:00")
	}
	if updatedAt.Valid {
		v.UpdatedAt = updatedAt.Time.Format("2006-01-02T15:04:05Z07:00")
	}
	return v, nil
}

// SetProcessing atualiza status para processing
func (r *VideoRepository) SetProcessing(ctx context.Context, videoID int) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE videos SET status = $1, updated_at = NOW() WHERE id = $2`,
		domain.StatusProcessing, videoID,
	)
	return err
}

// Atualiza status e caminho do ZIP de resultado
func (r *VideoRepository) SetCompleted(ctx context.Context, videoID int, resultZipPath string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE videos SET status = $1, result_zip_path = $2, error_message = NULL, updated_at = NOW() WHERE id = $3`,
		domain.StatusCompleted, resultZipPath, videoID,
	)
	return err
}

// Atualiza status e mensagem de erro
func (r *VideoRepository) SetFailed(ctx context.Context, videoID int, errorMessage string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE videos SET status = $1, error_message = $2, updated_at = NOW() WHERE id = $3`,
		domain.StatusFailed, errorMessage, videoID,
	)
	return err
}
