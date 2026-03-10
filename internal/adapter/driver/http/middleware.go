package http

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
)

// Headers repassados pelo API Gateway (Lambda Authorizer).
// O serviço não valida JWT
const (
	HeaderUserID    = "X-User-Id"
	HeaderUserEmail = "X-User-Email"
)

// Middleware de autorização: rejeita com 401 se X-User-Id estiver ausente ou inválido.
// Injeta user_id e user_email no context para os handlers.
func RequireUserID(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := userIDFromRequest(r)
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "missing or invalid X-User-Id header"})
			return
		}
		email := r.Header.Get(HeaderUserEmail)
		ctx := context.WithValue(r.Context(), contextKeyUserID, userID)
		ctx = context.WithValue(ctx, contextKeyUserEmail, email)
		next(w, r.WithContext(ctx))
	}
}

// RequireUserIDHandler é a versão do middleware que aceita http.Handler (para uso com chi).
func RequireUserIDHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := userIDFromRequest(r)
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "missing or invalid X-User-Id header"})
			return
		}
		email := r.Header.Get(HeaderUserEmail)
		ctx := context.WithValue(r.Context(), contextKeyUserID, userID)
		ctx = context.WithValue(ctx, contextKeyUserEmail, email)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func userIDFromRequest(r *http.Request) (int, bool) {
	s := r.Header.Get(HeaderUserID)
	if s == "" {
		return 0, false
	}
	id, err := strconv.Atoi(s)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}
