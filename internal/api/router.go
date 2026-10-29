package api

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"gokku/internal/authz"
)

// HandlerFunc is an API handler. A returned error is written in the API error shape.
type HandlerFunc func(w http.ResponseWriter, r *http.Request) error

// RouteInfo records how a route is protected (PERM-1).
type RouteInfo struct {
	Method string
	Path   string
	Action authz.Action // empty for public routes
	Public bool
}

// router wraps chi so that every route declares an action or is explicitly public.
type router struct {
	chi    chi.Router
	routes []RouteInfo
	log    *slog.Logger
	err    error // a registration mistake, returned by New
}

// Route registers a handler that requires action. Unauthenticated callers get 401,
// callers whose role is too low get 403.
func (rt *router) Route(method, path string, action authz.Action, h HandlerFunc) {
	if !authz.Known(action) {
		rt.err = errors.Join(rt.err, errors.New("api: route "+method+" "+path+" uses unknown action "+string(action)))
		return
	}
	rt.routes = append(rt.routes, RouteInfo{Method: method, Path: path, Action: action})
	rt.chi.Method(method, path, rt.wrap(func(w http.ResponseWriter, r *http.Request) error {
		p := authz.FromContext(r.Context())
		if p == nil {
			return errUnauthorized
		}
		if !authz.Can(p, action, authz.Resource{}) {
			return errForbidden
		}
		return h(w, r)
	}))
}

// Public registers a handler that needs no authentication.
func (rt *router) Public(method, path string, h HandlerFunc) {
	rt.routes = append(rt.routes, RouteInfo{Method: method, Path: path, Public: true})
	rt.chi.Method(method, path, rt.wrap(h))
}

func (rt *router) wrap(h HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := h(w, r)
		if err == nil {
			return
		}
		ae := toAPIError(err)
		if ae.Status >= http.StatusInternalServerError {
			rt.log.LogAttrs(r.Context(), slog.LevelError, "request failed",
				slog.String("method", r.Method), slog.String("path", r.URL.Path), slog.Any("error", err))
		}
		writeError(w, ae)
	}
}
