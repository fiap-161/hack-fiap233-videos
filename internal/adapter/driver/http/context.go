package http

import "context"

type contextKey string

const (
	contextKeyUserID    contextKey = "user_id"
	contextKeyUserEmail contextKey = "user_email"
)

// Retorna o user_id injetado pelo middleware (header X-User-Id do API Gateway)
func UserIDFromContext(ctx context.Context) (int, bool) {
	id, ok := ctx.Value(contextKeyUserID).(int)
	return id, ok
}

// Retorna o email injetado pelo middleware (header X-User-Email), quando disponível
func UserEmailFromContext(ctx context.Context) string {
	s, _ := ctx.Value(contextKeyUserEmail).(string)
	return s
}
