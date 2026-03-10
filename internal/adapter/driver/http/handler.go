package http

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/hack-fiap233/videos/internal/application"
	"github.com/hack-fiap233/videos/internal/domain"
)

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

// Não exige autenticação)
func (h *VideoHandler) Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := h.health.Ping(r.Context()); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "unhealthy", "db": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "service": "videos", "db": "connected"})
}

// List responde GET /videos — lista vídeos do usuário.
func (h *VideoHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "missing or invalid X-User-Id header"})
		return
	}
	list, err := h.service.ListByUser(r.Context(), userID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
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
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// Create responde POST — cria vídeo (só metadado)
func (h *VideoHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "missing or invalid X-User-Id header"})
		return
	}
	var input struct {
		Title       string `json:"title"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON"})
		return
	}
	v, err := h.service.CreateVideo(r.Context(), userID, input.Title, input.Description)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		if err == application.ErrTitleRequired {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "title is required"})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(domainToResponse(v))
}

// Upload trata POST /videos/upload (multipart): arquivo de vídeo + opcionais title, description.
func (h *VideoHandler) Upload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "missing or invalid X-User-Id header"})
		return
	}
	userEmail := UserEmailFromContext(r.Context())

	const maxFormMem = 32 << 20 // 32 MiB
	if err := r.ParseMultipartForm(maxFormMem); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid multipart form"})
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "missing or invalid file field"})
		return
	}
	defer file.Close()

	title := r.FormValue("title")
	if title == "" {
		title = header.Filename
	}
	description := r.FormValue("description")
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	v, err := h.service.UploadVideo(r.Context(), userID, userEmail, title, description, file, contentType)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(domainToResponse(v))
}

// GetByID  /videos/{id} — detalhe do vídeo (só do dono).
func (h *VideoHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "missing or invalid X-User-Id header"})
		return
	}
	idStr := chi.URLParam(r, "id")
	videoID, err := strconv.Atoi(idStr)
	if err != nil || videoID <= 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
		return
	}
	v, err := h.service.GetByIDForUser(r.Context(), userID, videoID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		if err == application.ErrVideoNotFound {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "video not found"})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(domainToResponse(v))
}

// Download responde GET /videos/{id}/download — arquivo ZIP (só se completed e dono).
func (h *VideoHandler) Download(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	videoID, err := strconv.Atoi(idStr)
	if err != nil || videoID <= 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
		return
	}
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "missing or invalid X-User-Id header"})
		return
	}
	reader, filename, err := h.service.DownloadResultZip(r.Context(), userID, videoID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		if err == application.ErrVideoNotFound {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "video not found"})
			return
		}
		if err == application.ErrInvalidStatus {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "video not ready for download (status must be completed)"})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	defer reader.Close()
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, reader)
}
