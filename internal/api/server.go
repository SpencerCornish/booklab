package api

import (
	"fmt"
	"io/fs"
	"net/http"

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
	stripe  *stripesvc.Service
	email   *emailsvc.Service
	webFS   fs.FS
}

func New(cfg *config.Config, queries *db.Queries, stripe *stripesvc.Service, email *emailsvc.Service, webFS fs.FS) *Server {
	return &Server{cfg: cfg, queries: queries, stripe: stripe, email: email, webFS: webFS}
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
	}))

	// API routes
	r.Route("/api", func(r chi.Router) {
		// Public
		r.Get("/settings/public", s.handleGetPublicSettings)
		r.Get("/availability", s.handleGetAvailability)
		r.Post("/bookings", s.handleCreateBooking)
		r.Get("/bookings/{token}", s.handleGetBooking)
		r.Post("/bookings/{token}/cancel", s.handleCancelBooking)

		// Admin
		r.Post("/admin/login", s.handleAdminLogin)
		r.Post("/admin/logout", s.handleAdminLogout)
		r.Group(func(r chi.Router) {
			r.Use(s.requireAdmin)
			r.Get("/admin/bookings", s.handleAdminListBookings)
			r.Patch("/admin/bookings/{id}", s.handleAdminUpdateBooking)
			r.Post("/admin/bookings/{id}/charge", s.handleAdminChargeBooking)
			r.Get("/admin/settings", s.handleAdminGetSettings)
			r.Put("/admin/settings", s.handleAdminUpdateSettings)
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
