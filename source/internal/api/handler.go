package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/sollie/tommedag/internal/avfallsor"
	"github.com/sollie/tommedag/internal/pickup"
)

// Handler serves the tommedag HTTP API.
type Handler struct {
	resolver Resolver
	logger   *slog.Logger
}

// NewHandler builds a Handler backed by resolver. If logger is nil, a
// no-op logger is used.
func NewHandler(resolver Resolver, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Handler{resolver: resolver, logger: logger}
}

// Healthz is a trivial liveness probe.
func (h *Handler) Healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// GetPickupDays resolves the ?address= query to a property and returns its
// pickup schedule as JSON.
func (h *Handler) GetPickupDays(w http.ResponseWriter, r *http.Request) {
	address := strings.TrimSpace(r.URL.Query().Get("address"))
	if address == "" {
		writeError(w, http.StatusBadRequest, "query parameter 'address' is required")
		return
	}

	ctx := r.Context()

	match, err := h.resolver.ResolveAddress(ctx, address)
	if err != nil {
		h.writeResolverError(w, "resolve address", address, err)
		return
	}

	cal, err := h.resolver.PickupCalendar(ctx, match.PropertyID)
	if err != nil {
		h.writeResolverError(w, "fetch calendar", address, err)
		return
	}

	writeJSON(w, http.StatusOK, pickup.NewResponse(address, match, cal))
}

func (h *Handler) writeResolverError(w http.ResponseWriter, op, address string, err error) {
	switch {
	case errors.Is(err, avfallsor.ErrAddressNotFound):
		writeError(w, http.StatusNotFound, "no matching address found")
	case errors.Is(err, avfallsor.ErrInvalidPropertyID):
		// A resolved address produced an unusable property id; treat as a
		// not-found from the caller's perspective.
		writeError(w, http.StatusNotFound, "no matching address found")
	case errors.Is(err, avfallsor.ErrUpstreamUnavailable):
		writeError(w, http.StatusBadGateway, "upstream service unavailable")
	default:
		h.logger.Error("unexpected error", "op", op, "address", address, "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, ErrorResponse{Error: msg})
}
