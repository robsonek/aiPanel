package database

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// Handler exposes HTTP handlers for database CRUD.
type Handler struct {
	svc *Service
}

// NewHandler creates database HTTP handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// HandleSiteDatabases serves POST/GET /api/sites/{siteID}/databases.
func (h *Handler) HandleSiteDatabases(w http.ResponseWriter, r *http.Request, siteID int64, actor string) {
	switch r.Method {
	case http.MethodGet:
		dbs, err := h.svc.ListDatabases(r.Context(), siteID)
		if err != nil {
			http.Error(w, "failed to list databases", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"databases": dbs})
	case http.MethodPost:
		var payload struct {
			DBName   string `json:"db_name"`
			DBEngine string `json:"db_engine"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&payload); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		res, err := h.svc.CreateDatabase(r.Context(), CreateDatabaseRequest{
			SiteID:   siteID,
			DBName:   payload.DBName,
			DBEngine: payload.DBEngine,
			Actor:    actor,
		})
		if err != nil {
			if isCreateDatabaseServiceUnavailable(err) {
				http.Error(w, err.Error(), http.StatusServiceUnavailable)
				return
			}
			if isCreateDatabaseBadRequest(err) {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			http.Error(w, "failed to create database", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusCreated, res)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleDatabaseEngines serves GET /api/databases/engines.
func (h *Handler) HandleDatabaseEngines(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	engines, err := h.svc.AvailableEngines(r.Context())
	if err != nil {
		http.Error(w, "failed to list database engines", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"engines": engines})
}

// HandleDatabaseByID serves DELETE /api/databases/{id}.
func (h *Handler) HandleDatabaseByID(w http.ResponseWriter, r *http.Request, id int64, actor string) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := h.svc.DeleteDatabase(r.Context(), id, actor); err != nil {
		if errors.Is(err, ErrDatabaseNotFound) {
			http.Error(w, "database not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to delete database", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ParseSiteIDFromDatabasesPath extracts site ID from "/api/sites/{siteID}/databases".
func ParseSiteIDFromDatabasesPath(path string) (int64, error) {
	trimmed := strings.TrimPrefix(path, "/api/sites/")
	trimmed = strings.TrimSpace(strings.Trim(trimmed, "/"))
	parts := strings.Split(trimmed, "/")
	if len(parts) < 2 {
		return 0, strconv.ErrSyntax
	}
	return strconv.ParseInt(parts[0], 10, 64)
}

// ParseDatabaseID extracts id from "/api/databases/{id}".
func ParseDatabaseID(path string) (int64, error) {
	trimmed := strings.TrimPrefix(path, "/api/databases/")
	trimmed = strings.TrimSpace(strings.Trim(trimmed, "/"))
	return strconv.ParseInt(trimmed, 10, 64)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func isCreateDatabaseBadRequest(err error) bool {
	if err == nil {
		return false
	}
	switch strings.TrimSpace(err.Error()) {
	case "site_id is required", "invalid database name", "invalid database engine", "site not found":
		return true
	default:
		return false
	}
}

func isCreateDatabaseServiceUnavailable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.TrimSpace(strings.ToLower(err.Error()))
	return strings.Contains(msg, "database engine") && strings.Contains(msg, "unavailable")
}
