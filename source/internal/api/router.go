package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// NewRouter wires the HTTP routes and middleware for the tommedag API.
func NewRouter(resolver Resolver, logger *slog.Logger) http.Handler {
	h := NewHandler(resolver, logger)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(15 * time.Second))

	r.Get("/healthz", h.Healthz)
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/pickup-days", h.GetPickupDays)
	})

	return r
}
