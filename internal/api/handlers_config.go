package api

import (
	"context"
	"maps"
	"net/http"
	"slices"

	"github.com/go-chi/chi/v5"

	"gokku/internal/audit"
	"gokku/internal/authz"
	"gokku/internal/dokku"
	"gokku/internal/reconcile"
	"gokku/internal/snapshot"
)

// configTarget returns the app of a config route, which needs Dokku.
func (a *API) configTarget(r *http.Request) (snapshot.App, error) {
	state := a.snap.State()
	app, ok := state.App(chi.URLParam(r, "name"))
	if !ok {
		return app, newError(http.StatusNotFound, ErrorCodeNotFound, "No app with this name on the server.")
	}
	if !state.Server.Reachable {
		return app, errDokkuUnreachable
	}
	return app, nil
}

// managedBy says which variables are set at their source (CFG-3): by a linked service,
// whose connection string they hold, or by the app's project, whose value they still hold.
func (a *API) managedBy(ctx context.Context, app snapshot.App, env map[string]string) (map[string]string, error) {
	out := map[string]string{}
	for _, s := range app.Services {
		dsn, err := a.dokku.ServiceDSN(ctx, s.Type, s.Name)
		if err != nil || dsn == "" {
			continue // the service may be gone; its variables are then the app's own
		}
		for k, v := range env {
			if v == dsn {
				out[k] = "service:" + s.Name
			}
		}
	}
	projectID, slug, err := a.appProject(ctx, app.Name)
	if err != nil || projectID == "" {
		return out, err
	}
	applied, err := a.db.AppProvenance(ctx, app.Name, projectID)
	if err != nil {
		return nil, err
	}
	for k, h := range applied {
		if v, ok := env[k]; ok && reconcile.Hash(v) == h {
			out[k] = "project:" + slug
		}
	}
	return out, nil
}

// appConfig reads the app's variables, masked (CFG-1) and with their source (CFG-3).
func (a *API) appConfig(ctx context.Context, app snapshot.App) (AppConfig, map[string]string, error) {
	env, err := a.dokku.Config(ctx, app.Name)
	if err != nil {
		return AppConfig{}, nil, err
	}
	managed, err := a.managedBy(ctx, app, env)
	if err != nil {
		return AppConfig{}, nil, err
	}
	out := AppConfig{Vars: make([]ConfigVar, 0, len(env))}
	for _, k := range slices.Sorted(maps.Keys(env)) {
		v := ConfigVar{Key: k, Masked: audit.LooksSecret(k)}
		if !v.Masked {
			value := env[k]
			v.Value = &value
		}
		if m, ok := managed[k]; ok {
			v.ManagedBy = &m
		}
		out.Vars = append(out.Vars, v)
	}
	return out, managed, nil
}

func (a *API) getAppConfig(w http.ResponseWriter, r *http.Request) error {
	app, err := a.configTarget(r)
	if err != nil {
		return err
	}
	out, _, err := a.appConfig(r.Context(), app)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// updateAppConfig sets and unsets variables with --no-restart, then restarts once as a
// job if asked (CFG-4). Variables managed at their source are refused (CFG-3).
func (a *API) updateAppConfig(w http.ResponseWriter, r *http.Request) error {
	var req UpdateConfigRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	app, err := a.configTarget(r)
	if err != nil {
		return err
	}
	ctx := r.Context()
	set := map[string]string{}
	if req.Set != nil {
		set = *req.Set
	}
	var unset []string
	if req.Unset != nil {
		unset = *req.Unset
	}
	fields := map[string]string{}
	for k := range set {
		if !dokku.ValidConfigKey(k) {
			fields[k] = "Use letters, digits and underscores, not starting with a digit."
		}
	}
	for _, k := range unset {
		if !dokku.ValidConfigKey(k) {
			fields[k] = "Use letters, digits and underscores, not starting with a digit."
		}
		if _, both := set[k]; both {
			fields[k] = "A key cannot be set and removed at once."
		}
	}
	if len(fields) > 0 {
		return validationError(fields)
	}

	env, err := a.dokku.Config(ctx, app.Name)
	if err != nil {
		return err
	}
	managed, err := a.managedBy(ctx, app, env)
	if err != nil {
		return err
	}
	for _, k := range append(slices.Collect(maps.Keys(set)), unset...) {
		if source, ok := managed[k]; ok {
			fields[k] = "Set by " + source + "; change it at its source."
		}
	}
	if len(fields) > 0 {
		ae := newError(http.StatusBadRequest, ErrorCodeManagedVariable, "Some variables are managed elsewhere and are read-only here.")
		ae.Fields = fields
		return ae
	}

	_, projectSlug, _ := a.appProject(ctx, app.Name)
	entry := audit.Entry{
		Action: "config:set", TargetType: "app", TargetName: app.Name, ProjectSlug: projectSlug,
		Detail: map[string]any{"set": slices.Sorted(maps.Keys(set)), "unset": unset, "restart": req.Restart != nil && *req.Restart},
	}
	p := authz.FromContext(ctx)
	if err := a.dokku.SetConfig(ctx, app.Name, set); err != nil {
		entry.Result = audit.ResultFail
		a.audit.Record(ctx, p, entry)
		return err
	}
	if err := a.dokku.UnsetConfig(ctx, app.Name, unset); err != nil {
		entry.Result = audit.ResultFail
		a.audit.Record(ctx, p, entry)
		return err
	}
	a.audit.Record(ctx, p, entry)

	out := ConfigUpdated{}
	if req.Restart != nil && *req.Restart && (len(set) > 0 || len(unset) > 0) {
		job, err := a.submitLifecycle(ctx, dokku.ActionRestart, app.Name)
		if err != nil {
			return err
		}
		out.JobId = &job.ID
	}
	if out.Config, _, err = a.appConfig(ctx, app); err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// revealAppConfig returns the values of the asked keys, audited with the keys only (CFG-2).
func (a *API) revealAppConfig(w http.ResponseWriter, r *http.Request) error {
	var req RevealRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	if len(req.Keys) == 0 || len(req.Keys) > 100 {
		return validationError(map[string]string{"keys": "Ask for 1 to 100 keys."})
	}
	app, err := a.configTarget(r)
	if err != nil {
		return err
	}
	ctx := r.Context()
	env, err := a.dokku.Config(ctx, app.Name)
	if err != nil {
		return err
	}
	out := RevealedValues{Values: map[string]string{}}
	for _, k := range req.Keys {
		if v, ok := env[k]; ok {
			out.Values[k] = v
		}
	}
	_, projectSlug, _ := a.appProject(ctx, app.Name)
	a.audit.Record(ctx, authz.FromContext(ctx), audit.Entry{
		Action: "config:reveal", TargetType: "app", TargetName: app.Name, ProjectSlug: projectSlug,
		Detail: map[string]any{"names": slices.Sorted(maps.Keys(out.Values))},
	})
	writeJSON(w, http.StatusOK, out)
	return nil
}
