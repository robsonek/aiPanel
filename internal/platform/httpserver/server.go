// Package httpserver provides HTTP server bootstrap and routing.
package httpserver

import (
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	aipanel "github.com/robsonek/aiPanel"
	"github.com/robsonek/aiPanel/internal/modules/database"
	"github.com/robsonek/aiPanel/internal/modules/hosting"
	"github.com/robsonek/aiPanel/internal/modules/iam"
	"github.com/robsonek/aiPanel/internal/platform/config"
	"github.com/robsonek/aiPanel/internal/platform/middleware"
)

// NewHandler creates the root HTTP handler for panel API and frontend.
func NewHandler(
	cfg config.Config,
	log *slog.Logger,
	iamSvc *iam.Service,
	hostingSvc *hosting.Service,
	databaseSvc *database.Service,
) http.Handler {
	mux := http.NewServeMux()
	secureCookie := !strings.EqualFold(cfg.Env, "dev") && !strings.EqualFold(cfg.Env, "test")
	hostingHandler := hosting.NewHandler(hostingSvc)
	databaseHandler := database.NewHandler(databaseSvc)

	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("/api/auth/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		session, err := iamSvc.Login(r.Context(), req.Email, req.Password)
		if err != nil {
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     cfg.SessionCookieName,
			Value:    session.Token,
			Path:     "/",
			HttpOnly: true,
			Secure:   secureCookie,
			SameSite: http.SameSiteLaxMode,
			Expires:  session.ExpiresAt,
		})
		writeJSON(w, http.StatusOK, map[string]any{
			"user": map[string]any{
				"id":    session.User.ID,
				"email": session.User.Email,
				"role":  session.User.Role,
			},
		})
	})

	mux.Handle("/api/auth/logout", requireAuth(iamSvc, cfg.SessionCookieName, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		u, ok := userFromContext(r.Context())
		if ok {
			log.Info("logout", "user_id", u.ID, "email", u.Email)
		}
		_ = iamSvc.Logout(r.Context(), readSessionToken(r, cfg.SessionCookieName))
		http.SetCookie(w, &http.Cookie{
			Name:     cfg.SessionCookieName,
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			Secure:   secureCookie,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   -1,
		})
		w.WriteHeader(http.StatusNoContent)
	})))

	mux.Handle("/api/auth/me", requireAuth(iamSvc, cfg.SessionCookieName, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		u, ok := userFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"user": u})
	})))

	mux.Handle("/api/admin/ping", requireAuth(iamSvc, cfg.SessionCookieName, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		u, ok := userFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if u.Role != "admin" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})))

	if hostingSvc != nil {
		mux.Handle("/api/sites", requireAdmin(iamSvc, cfg.SessionCookieName, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, _ := userFromContext(r.Context())
			hostingHandler.HandleSites(w, r, u.Email)
		})))

		mux.Handle("/api/sites/", requireAdmin(iamSvc, cfg.SessionCookieName, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, _ := userFromContext(r.Context())
			if strings.HasSuffix(strings.Trim(r.URL.Path, "/"), "databases") {
				if databaseSvc == nil {
					http.Error(w, "database service unavailable", http.StatusServiceUnavailable)
					return
				}
				siteID, err := database.ParseSiteIDFromDatabasesPath(r.URL.Path)
				if err != nil {
					http.Error(w, "invalid site id", http.StatusBadRequest)
					return
				}
				databaseHandler.HandleSiteDatabases(w, r, siteID, u.Email)
				return
			}
			siteID, err := hosting.ParseSiteID(r.URL.Path)
			if err != nil {
				http.Error(w, "invalid site id", http.StatusBadRequest)
				return
			}
			hostingHandler.HandleSiteByID(w, r, siteID, u.Email)
		})))
	}

	if databaseSvc != nil {
		mux.Handle("/api/databases/", requireAdmin(iamSvc, cfg.SessionCookieName, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, _ := userFromContext(r.Context())
			id, err := database.ParseDatabaseID(r.URL.Path)
			if err != nil {
				http.Error(w, "invalid database id", http.StatusBadRequest)
				return
			}
			databaseHandler.HandleDatabaseByID(w, r, id, u.Email)
		})))
	}

	frontend := frontendHandler(cfg, log)
	mux.Handle("/", frontend)

	return middleware.Chain(
		mux,
		middleware.RequestIDMiddleware,
		middleware.LoggingMiddleware(log),
		middleware.CORSMiddleware,
		middleware.RecoveryMiddleware(log),
	)
}

type userCtxKey string

const authUserKey userCtxKey = "auth_user"

func requireAuth(iamSvc *iam.Service, cookieName string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := readSessionToken(r, cookieName)
		user, err := iamSvc.Authenticate(r.Context(), token)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), authUserKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func requireAdmin(iamSvc *iam.Service, cookieName string, next http.Handler) http.Handler {
	return requireAuth(iamSvc, cookieName, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := userFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if u.Role != "admin" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	}))
}

func userFromContext(ctx context.Context) (iam.User, bool) {
	v, ok := ctx.Value(authUserKey).(iam.User)
	return v, ok
}

func readSessionToken(r *http.Request, cookieName string) string {
	if auth := strings.TrimSpace(r.Header.Get("Authorization")); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	c, err := r.Cookie(cookieName)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(c.Value)
}

func frontendHandler(cfg config.Config, log *slog.Logger) http.Handler {
	if strings.EqualFold(cfg.Env, "dev") && cfg.DevFrontendProxy != "" {
		targetURL, err := url.Parse(cfg.DevFrontendProxy)
		if err == nil {
			proxy := httputil.NewSingleHostReverseProxy(targetURL)
			orig := proxy.ErrorHandler
			proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, e error) {
				log.Error("vite proxy error", "error", e.Error())
				if orig != nil {
					orig(w, r, e)
					return
				}
				http.Error(w, "frontend proxy unavailable", http.StatusBadGateway)
			}
			return proxy
		}
	}
	return embeddedFrontend()
}

func embeddedFrontend() http.Handler {
	distFS, err := fs.Sub(aipanel.FrontendFS, "web/dist")
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "frontend unavailable", http.StatusServiceUnavailable)
		})
	}
	fileServer := http.FileServer(http.FS(distFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/health" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path == "/" {
			fileServer.ServeHTTP(w, r)
			return
		}
		trimmed := strings.TrimPrefix(r.URL.Path, "/")
		if _, err := fs.Stat(distFS, trimmed); err != nil {
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			fileServer.ServeHTTP(w, r2)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
