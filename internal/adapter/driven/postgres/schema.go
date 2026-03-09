package postgres

import "database/sql"

// CreateTableIfNotExists cria a tabela videos para ambiente local.
// Em produção usa as migrations da infra (hack-fiap233-infra/migrations/videos/).
func CreateTableIfNotExists(db *sql.DB) error {
	query := `CREATE TABLE IF NOT EXISTS videos (
		id               SERIAL PRIMARY KEY,
		user_id          INTEGER NOT NULL,
		title            TEXT NOT NULL,
		description      TEXT NOT NULL DEFAULT '',
		status           TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'completed', 'failed')),
		storage_key      TEXT,
		result_zip_path  TEXT,
		error_message    TEXT,
		created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`
	if _, err := db.Exec(query); err != nil {
		return err
	}
	_, err := db.Exec("CREATE INDEX IF NOT EXISTS videos_user_id_idx ON videos (user_id)")
	return err
}
