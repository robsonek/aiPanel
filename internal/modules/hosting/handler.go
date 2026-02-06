package hosting

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// Handler exposes HTTP handlers for site CRUD.
type Handler struct {
	svc *Service
}

// NewHandler creates hosting HTTP handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// HandleSites serves POST/GET /api/sites.
func (h *Handler) HandleSites(w http.ResponseWriter, r *http.Request, actor string) {
	switch r.Method {
	case http.MethodGet:
		sites, err := h.svc.ListSites(r.Context())
		if err != nil {
			http.Error(w, "failed to list sites", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"sites": sites})
	case http.MethodPost:
		var req CreateSiteRequest
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		req.Actor = actor
		site, err := h.svc.CreateSite(r.Context(), req)
		if err != nil {
			errMsg := strings.ToLower(err.Error())
			if strings.Contains(errMsg, "invalid") ||
				strings.Contains(errMsg, "required") ||
				strings.Contains(errMsg, "not installed") ||
				strings.Contains(errMsg, "already exists") {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			http.Error(w, "failed to create site: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"site": site})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleSiteByID serves GET/DELETE /api/sites/{id}.
func (h *Handler) HandleSiteByID(w http.ResponseWriter, r *http.Request, id int64, actor string) {
	switch r.Method {
	case http.MethodGet:
		site, err := h.svc.GetSite(r.Context(), id)
		if err != nil {
			if errors.Is(err, ErrSiteNotFound) {
				http.Error(w, "site not found", http.StatusNotFound)
				return
			}
			http.Error(w, "failed to get site", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"site": site})
	case http.MethodDelete:
		if err := h.svc.DeleteSite(r.Context(), id, actor); err != nil {
			if errors.Is(err, ErrSiteNotFound) {
				http.Error(w, "site not found", http.StatusNotFound)
				return
			}
			http.Error(w, "failed to delete site", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// ParseSiteID extracts id from "/api/sites/{id}".
func ParseSiteID(path string) (int64, error) {
	idRaw := strings.TrimPrefix(path, "/api/sites/")
	idRaw = strings.TrimSpace(strings.Trim(idRaw, "/"))
	return strconv.ParseInt(idRaw, 10, 64)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
