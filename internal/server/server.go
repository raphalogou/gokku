// Package server builds Gokku's HTTP handler: middleware, API routes and the SPA fallback.
package server

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Options are the dependencies of the HTTP handler.
type Options struct {
	Logger  *slog.Logger
	Version string
	// Static is the built SPA, rooted at the directory holding index.html.
	Static fs.FS
}

// New returns the root handler. Order: RequestID, Recover, Logger, SecurityHeaders (DESIGN.md, 3.1).
func New(opts Options) (http.Handler, error) {
	spa, err := newSPAHandler(opts.Static)
	if err != nil {
		return nil, fmt.Errorf("load web app: %w", err)
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(recoverer(opts.Logger))
	r.Use(requestLogger(opts.Logger))
	r.Use(securityHeaders(spa.csp))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": opts.Version})
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
			writeError(w, http.StatusNotFound, "not_found", "no such endpoint")
		})
		r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		})
	})

	r.NotFound(spa.ServeHTTP)
	r.MethodNotAllowed(spa.ServeHTTP)
	return r, nil
}

// errorBody is the single API error shape (SPEC.md, section 8).
type errorBody struct {
	Error struct {
		Code    string            `json:"code"`
		Message string            `json:"message"`
		Fields  map[string]string `json:"fields,omitempty"`
	} `json:"error"`
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	var body errorBody
	body.Error.Code = code
	body.Error.Message = message
	writeJSON(w, status, body)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	// The status line is already sent; an encoding error can only be a broken connection.
	_ = json.NewEncoder(w).Encode(v)
}
