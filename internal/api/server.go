package api

import (
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/spencercornish/booklab/internal/config"
	"github.com/spencercornish/booklab/internal/db"
	emailsvc "github.com/spencercornish/booklab/internal/email"
	stripesvc "github.com/spencercornish/booklab/internal/stripe"
)

type Server struct {
	cfg     *config.Config
	queries *db.Queries
	stripe  stripesvc.Client
	email   *emailsvc.Service
	webFS   fs.FS
	logger  *slog.Logger
}

func New(cfg *config.Config, queries *db.Queries, stripe stripesvc.Client, email *emailsvc.Service, webFS fs.FS, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	return &Server{
		cfg: cfg, queries: queries, stripe: stripe, email: email, webFS: webFS,
		logger: log.With("component", "api"),
	}
}

func (s *Server) slogRequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		status := ww.Status()
		if status == 0 {
			status = 200
		}
		attrs := []any{
			"method", r.Method,
			"path", r.URL.Path,
			"status", status,
			"duration_ms", time.Since(start).Milliseconds(),
			"remote_addr", r.RemoteAddr,
		}
		switch {
		case status >= 500:
			s.logger.Error("http_request", attrs...)
		case status >= 400:
			s.logger.Warn("http_request", attrs...)
		default:
			s.logger.Info("http_request", attrs...)
		}
	})
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()

	r.Use(s.slogRequestLogger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   s.cfg.AllowedCORSOrigins(),
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		AllowCredentials: true,
	}))

	// API routes
	r.Route("/api", func(r chi.Router) {
		// Public
		r.Get("/settings/public", s.handleGetPublicSettings)
		r.Get("/terms", s.handleGetTerms)
		r.Get("/privacy", s.handleGetPrivacy)
		r.Get("/availability", s.handleGetAvailability)
		r.Post("/bookings/prepare", s.handlePrepareBooking)
		r.Post("/bookings", s.handleCreateBooking)
		r.Get("/bookings/{token}", s.handleGetBooking)
		r.Post("/bookings/{token}/cancel", s.handleCancelBooking)

		// Admin
		r.Post("/admin/login", s.handleAdminLogin)
		r.Post("/admin/logout", s.handleAdminLogout)
		r.Group(func(r chi.Router) {
			r.Use(s.requireAdmin)
			r.Use(s.csrfProtect)
			r.Get("/admin/bookings", s.handleAdminListBookings)
			r.Patch("/admin/bookings/{id}", s.handleAdminUpdateBooking)
			r.Post("/admin/bookings/{id}/charge", s.handleAdminChargeBooking)
			r.Get("/admin/settings", s.handleAdminGetSettings)
			r.Put("/admin/settings", s.handleAdminUpdateSettings)
			r.Get("/admin/insights", s.handleAdminGetInsights)
			r.Get("/admin/users", s.handleAdminListUsers)
			r.Post("/admin/users", s.handleAdminCreateUser)
			r.Put("/admin/users/me/password", s.handleAdminChangePassword)
			r.Delete("/admin/users/{username}", s.handleAdminDeleteUser)
			r.Get("/admin/closures", s.handleAdminListClosures)
			r.Post("/admin/closures", s.handleAdminCreateClosure)
			r.Put("/admin/closures/{id}", s.handleAdminUpdateClosure)
			r.Delete("/admin/closures/{id}", s.handleAdminDeleteClosure)
		})
	})

	// Serve SPA for all other routes
	r.Get("/*", s.handleSPA)

	return r
}

func (s *Server) handleSPA(w http.ResponseWriter, r *http.Request) {
	if s.webFS == nil {
		s.logger.Warn("spa_not_available", "path", r.URL.Path)
		http.Error(w, "frontend not built", http.StatusServiceUnavailable)
		return
	}
	fileServer := http.FileServer(http.FS(s.webFS))

	path := r.URL.Path
	if path == "/" {
		path = "/index.html"
	}
	f, err := s.webFS.Open(path[1:])
	if err != nil {
		http.ServeFileFS(w, r, s.webFS, "index.html")
		return
	}
	f.Close()
	fileServer.ServeHTTP(w, r)
}

func (s *Server) Addr() string {
	return fmt.Sprintf(":%d", s.cfg.Port)
}
