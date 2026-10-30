// Package api implements the /api/v1 REST endpoints described in api/openapi.yaml.
package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"gokku/internal/audit"
	"gokku/internal/auth"
	"gokku/internal/authz"
	"gokku/internal/events"
	"gokku/internal/snapshot"
	"gokku/internal/store"
)

//go:generate echo "run 'task gen' from the repository root"

// API holds the dependencies of the handlers.
type API struct {
	auth       *auth.Service
	db         *store.DB
	audit      *audit.Recorder
	log        *slog.Logger
	snap       *snapshot.Refresher
	hub        *events.Hub
	serverName string
	dokkuHost  string
	heartbeat  time.Duration
	handler    http.Handler
	routes     []RouteInfo
}

// Options are the dependencies of the API.
type Options struct {
	Auth     *auth.Service
	DB       *store.DB
	Audit    *audit.Recorder
	Log      *slog.Logger
	Snapshot *snapshot.Refresher
	Hub      *events.Hub
	// ServerName is shown in the UI, for example the host name of the Dokku server.
	ServerName string
	// DokkuHost is the host in git remote instructions; empty means the request host.
	DokkuHost string
	// Heartbeat is the SSE heartbeat interval; zero means DefaultHeartbeat (EVT-3).
	Heartbeat time.Duration
}

// New builds the API handler, to be mounted at /api/v1.
func New(opts Options) (*API, error) {
	a := &API{
		auth: opts.Auth, db: opts.DB, audit: opts.Audit, log: opts.Log, snap: opts.Snapshot, hub: opts.Hub,
		serverName: opts.ServerName, dokkuHost: opts.DokkuHost, heartbeat: opts.Heartbeat,
	}
	if a.heartbeat == 0 {
		a.heartbeat = DefaultHeartbeat
	}

	r := chi.NewRouter()
	r.Use(a.authenticate, csrf)
	r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, newError(http.StatusNotFound, ErrorCodeNotFound, "No such endpoint."))
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, newError(http.StatusMethodNotAllowed, ErrorCodeMethodNotAllowed, "Method not allowed."))
	})

	rt := &router{chi: r, log: opts.Log}
	a.register(rt)
	if rt.err != nil {
		return nil, rt.err
	}
	a.handler = r
	a.routes = rt.routes
	return a, nil
}

// ServeHTTP implements http.Handler.
func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) { a.handler.ServeHTTP(w, r) }

// Routes lists every registered route with its protection, for tests.
func (a *API) Routes() []RouteInfo { return a.routes }

func (a *API) register(r *router) {
	r.Public(http.MethodGet, "/setup/status", a.getSetupStatus)
	r.Public(http.MethodPost, "/setup", a.completeSetup)

	r.Public(http.MethodPost, "/auth/login", a.login)
	r.Public(http.MethodPost, "/auth/logout", a.logout)
	r.Public(http.MethodGet, "/auth/invites/{token}", a.getInvite)
	r.Public(http.MethodPost, "/auth/accept-invite", a.acceptInvite)

	r.Route(http.MethodGet, "/me", authz.MeView, a.getMe)
	r.Route(http.MethodPatch, "/me", authz.MeUpdate, a.updateMe)
	r.Route(http.MethodPost, "/me/password", authz.MeUpdate, a.changePassword)
	r.Route(http.MethodGet, "/me/sessions", authz.MeView, a.listMySessions)
	r.Route(http.MethodDelete, "/me/sessions/{id}", authz.MeUpdate, a.revokeMySession)

	r.Route(http.MethodGet, "/users", authz.UserManage, a.listUsers)
	r.Route(http.MethodPost, "/users", authz.UserManage, a.inviteUser)
	r.Route(http.MethodPatch, "/users/{id}", authz.UserManage, a.updateUser)
	r.Route(http.MethodDelete, "/users/{id}", authz.UserManage, a.deleteUser)
	r.Route(http.MethodPost, "/users/{id}/invite", authz.UserManage, a.resetInvite)

	r.Route(http.MethodGet, "/tokens", authz.TokenManage, a.listTokens)
	r.Route(http.MethodPost, "/tokens", authz.TokenManage, a.createToken)
	r.Route(http.MethodDelete, "/tokens/{id}", authz.TokenManage, a.revokeToken)

	r.Route(http.MethodGet, "/server", authz.PluginView, a.getServer)
	r.Route(http.MethodPost, "/server/rescan", authz.ServerRescan, a.rescanServer)
	r.Route(http.MethodGet, "/plugins", authz.PluginView, a.listPlugins)
	r.Route(http.MethodGet, "/events", authz.AppView, a.streamEvents)

	r.Route(http.MethodGet, "/projects", authz.ProjectView, a.listProjects)
	r.Route(http.MethodPost, "/projects", authz.ProjectCreate, a.createProject)
	r.Route(http.MethodGet, "/projects/{slug}", authz.ProjectView, a.getProject)
	r.Route(http.MethodPatch, "/projects/{slug}", authz.ProjectUpdate, a.updateProject)
	r.Route(http.MethodDelete, "/projects/{slug}", authz.ProjectDelete, a.deleteProject)
	r.Route(http.MethodPut, "/projects/{slug}/apps/{app}", authz.ProjectMembersWrite, a.addProjectApp)
	r.Route(http.MethodDelete, "/projects/{slug}/apps/{app}", authz.ProjectMembersWrite, a.removeProjectApp)
	r.Route(http.MethodPut, "/projects/{slug}/services/{type}/{name}", authz.ProjectMembersWrite, a.addProjectService)
	r.Route(http.MethodDelete, "/projects/{slug}/services/{type}/{name}", authz.ProjectMembersWrite, a.removeProjectService)

	r.Route(http.MethodGet, "/apps", authz.AppView, a.listApps)
	r.Route(http.MethodGet, "/apps/{name}", authz.AppView, a.getApp)

	r.Route(http.MethodGet, "/services", authz.ServiceView, a.listServices)
}
