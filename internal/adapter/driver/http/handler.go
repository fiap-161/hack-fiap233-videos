package http

import (
	"encoding/json"
	"net/http"

	"github.com/hack-fiap233/videos/internal/application"
	"github.com/hack-fiap233/videos/internal/domain"
)

// Adapter driver HTTP: traduz request/response e chama o use case.
type VideoHandler struct {
	service *application.VideoService
	health  application.HealthChecker
}

func NewVideoHandler(service *application.VideoService, health application.HealthChecker) *VideoHandler {
	return &VideoHandler{service: service, health: health}
}

type videoResponse struct {
	ID            int    `json:"id"`
	UserID        int    `json:"user_id"`
	Title         string `json:"title"`
	Description   string `json:"description"`
	Status        string `json:"status"`
	StorageKey    string `json:"storage_key,omitempty"`
	ResultZipPath string `json:"result_zip_path,omitempty"`
	ErrorMessage  string `json:"error_message,omitempty"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

func domainToResponse(v domain.Video) videoResponse {
	return videoResponse{
		ID:            v.ID,
		UserID:        v.UserID,
		Title:         v.Title,
		Description:   v.Description,
		Status:        v.Status,
		StorageKey:    v.StorageKey,
		ResultZipPath: v.ResultZipPath,
		ErrorMessage:  v.ErrorMessage,
		CreatedAt:     v.CreatedAt,
		UpdatedAt:     v.UpdatedAt,
	}
}

// Não exige autenticação).
func (h *VideoHandler) Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := h.health.Ping(r.Context()); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "unhealthy", "db": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "service": "videos", "db": "connected"})
}

// Videos despacha GET (listar) e POST (criar); método não permitido retorna 405.
func (h *VideoHandler) Videos(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case http.MethodGet:
		h.listVideos(w, r)
	case http.MethodPost:
		h.createVideo(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
	}
}

func (h *VideoHandler) listVideos(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "missing or invalid X-User-Id header"})
		return
	}
	list, err := h.service.ListByUser(r.Context(), userID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	resp := make([]videoResponse, len(list))
	for i := range list {
		resp[i] = domainToResponse(list[i])
	}
	if resp == nil {
		resp = []videoResponse{}
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *VideoHandler) createVideo(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "missing or invalid X-User-Id header"})
		return
	}
	var input struct {
		Title       string `json:"title"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON"})
		return
	}
	v, err := h.service.CreateVideo(r.Context(), userID, input.Title, input.Description)
	if err != nil {
		if err == application.ErrTitleRequired {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "title is required"})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(domainToResponse(v))
}
