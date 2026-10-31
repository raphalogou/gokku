package api

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"slices"
	"strings"

	"github.com/go-chi/chi/v5"

	"gokku/internal/audit"
	"gokku/internal/authz"
	"gokku/internal/dokku"
	"gokku/internal/jobs"
	"gokku/internal/projects"
	"gokku/internal/store"
)

const envJobKind = "project.env"

// memberApps returns the project's apps that exist in the snapshot, sorted.
func (a *API) memberApps(ctx context.Context, projectID string) ([]string, error) {
	v, err := a.loadView(ctx)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, app := range v.projectApps(projectID) {
		out = append(out, app.Name)
	}
	return out, nil
}

// projectEnv builds the shared variables with their status per app (SHV-6, SHV-7). When
// Dokku cannot be read, the statuses are left out and reachable is false.
func (a *API) projectEnv(ctx context.Context, p store.Project) (ProjectEnv, error) {
	vars, err := a.env.Vars(ctx, p.ID)
	if err != nil {
		return ProjectEnv{}, err
	}
	apps, err := a.memberApps(ctx, p.ID)
	if err != nil {
		return ProjectEnv{}, err
	}
	values := make(map[string]string, len(vars))
	for _, v := range vars {
		values[v.Key] = v.Value
	}
	out := ProjectEnv{Vars: make([]SharedVar, 0, len(vars)), Apps: nonNil(apps), Reachable: a.snap.State().Server.Reachable}
	statuses := map[string]map[string]string{}
	if out.Reachable {
		got, err := a.env.Statuses(ctx, p.ID, values, apps)
		if err != nil {
			out.Reachable = false
		} else {
			for app, keys := range got {
				statuses[app] = map[string]string{}
				for k, s := range keys {
					statuses[app][k] = string(s)
				}
			}
		}
	}
	for _, v := range vars {
		sv := SharedVar{Key: v.Key, Secret: v.Secret, Masked: v.Secret, UpdatedAt: timePtr(v.UpdatedAt), Apps: []AppEnvStatus{}}
		if !v.Secret {
			value := v.Value
			sv.Value = &value
		}
		for _, app := range apps {
			if s, ok := statuses[app][v.Key]; ok {
				sv.Apps = append(sv.Apps, AppEnvStatus{App: app, Status: EnvStatus(s)})
			}
		}
		out.Vars = append(out.Vars, sv)
	}
	return out, nil
}

func (a *API) getProjectEnv(w http.ResponseWriter, r *http.Request) error {
	p, err := a.db.ProjectBySlug(r.Context(), chi.URLParam(r, "slug"))
	if err != nil {
		return projectNotFound(err)
	}
	out, err := a.projectEnv(r.Context(), p)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// putProjectEnv replaces the shared variables (SHV-4, SHV-5) and applies them as a job when
// asked ("Save and apply", SHV-6).
func (a *API) putProjectEnv(w http.ResponseWriter, r *http.Request) error {
	var req PutProjectEnvRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	ctx := r.Context()
	p, err := a.db.ProjectBySlug(ctx, chi.URLParam(r, "slug"))
	if err != nil {
		return projectNotFound(err)
	}
	current, err := a.env.Vars(ctx, p.ID)
	if err != nil {
		return err
	}
	stored := map[string]projects.Var{}
	for _, v := range current {
		stored[v.Key] = v
	}
	datastores := a.snap.State().Server.Capabilities.Datastores
	fields := map[string]string{}
	vars := make([]projects.Var, 0, len(req.Vars))
	seen := map[string]bool{}
	for _, in := range req.Vars {
		switch {
		case !projects.ValidKey(in.Key):
			fields[in.Key] = "Use upper-case letters, digits and underscores, not starting with a digit."
			continue
		case seen[in.Key]:
			fields[in.Key] = "This key is listed twice."
			continue
		}
		seen[in.Key] = true
		if why := projects.Reserved(in.Key, datastores); why != "" {
			fields[in.Key] = why
			continue
		}
		v := projects.Var{Key: in.Key, Secret: audit.LooksSecret(in.Key)}
		if in.Secret != nil {
			v.Secret = *in.Secret
		}
		switch old, ok := stored[in.Key]; {
		case in.Value != nil:
			v.Value = *in.Value
		case ok:
			v.Value = old.Value
		default:
			fields[in.Key] = "A new variable needs a value."
			continue
		}
		if len(v.Value) > 65536 {
			fields[in.Key] = "Use at most 64 KiB."
			continue
		}
		vars = append(vars, v)
	}
	if len(fields) > 0 {
		reserved := false
		for k := range fields {
			reserved = reserved || projects.Reserved(k, datastores) != ""
		}
		code := ErrorCodeValidationFailed
		if reserved {
			code = ErrorCodeReservedKey
		}
		ae := newError(http.StatusBadRequest, code, "Some variables cannot be saved.")
		ae.Fields = fields
		return ae
	}

	apply := req.Apply != nil && *req.Apply
	if apply && !a.snap.State().Server.Reachable {
		return errDokkuUnreachable
	}
	actor := authz.FromContext(ctx)
	if err := a.env.Save(ctx, p.ID, vars, actor.UserID); err != nil {
		return err
	}
	var removed []string
	for k := range stored {
		if !seen[k] {
			removed = append(removed, k)
		}
	}
	slices.Sort(removed)
	a.audit.Record(ctx, actor, audit.Entry{
		Action: "projects:env-set", TargetType: "project", TargetName: p.Slug, ProjectSlug: p.Slug,
		Detail: map[string]any{"names": slices.Sorted(maps.Keys(seen)), "removed": removed},
	})
	a.projectChanged(p.Slug)

	out := ProjectEnvSaved{}
	if apply {
		job, err := a.submitEnvApply(ctx, p, req.Restart != nil && *req.Restart)
		if err != nil {
			return err
		}
		out.JobId = &job.ID
	}
	if out.Env, err = a.projectEnv(ctx, p); err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (a *API) applyProjectEnv(w http.ResponseWriter, r *http.Request) error {
	var req ApplyEnvRequest
	if r.ContentLength != 0 {
		if err := decode(r, &req); err != nil {
			return err
		}
	}
	ctx := r.Context()
	p, err := a.db.ProjectBySlug(ctx, chi.URLParam(r, "slug"))
	if err != nil {
		return projectNotFound(err)
	}
	if !a.snap.State().Server.Reachable {
		return errDokkuUnreachable
	}
	job, err := a.submitEnvApply(ctx, p, req.Restart != nil && *req.Restart)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusAccepted, JobRef{JobId: job.ID})
	return nil
}

// submitEnvApply writes the shared variables into every member app as one job, with one
// step per app and, if asked, a last step restarting the apps that changed (SHV-3).
func (a *API) submitEnvApply(ctx context.Context, p store.Project, restart bool) (store.Job, error) {
	apps, err := a.memberApps(ctx, p.ID)
	if err != nil {
		return store.Job{}, err
	}
	steps := make([]string, 0, len(apps)+1)
	for _, app := range apps {
		steps = append(steps, "Apply to "+app)
	}
	if restart {
		steps = append(steps, "Restart changed apps")
	}
	if len(steps) == 0 {
		steps = []string{"Apply"}
	}
	actor := authz.FromContext(ctx)
	return a.jobs.Submit(ctx, jobs.Spec{
		Kind: envJobKind, Title: "Apply shared variables to " + p.Name, ProjectID: p.ID, Actor: actor, Steps: steps,
		Run: func(ctx context.Context, t *jobs.Task) error {
			vars, err := a.env.Values(ctx, p.ID)
			if err != nil {
				return err
			}
			log := func(s string) { t.Write("stdout", s) }
			var changed []string
			var failed []string
			for i, app := range apps {
				t.Step(i)
				ok, err := a.env.Apply(ctx, p.ID, app, vars, log)
				if err != nil {
					t.Write("stderr", app+": "+err.Error())
					failed = append(failed, app)
					continue
				}
				if ok {
					changed = append(changed, app)
				}
			}
			if restart {
				t.Step(len(apps))
				for _, app := range changed {
					if err := a.dokku.Lifecycle(ctx, dokku.ActionRestart, app, t.Line); err != nil {
						t.Write("stderr", err.Error())
						failed = append(failed, app)
					}
				}
			}
			a.snap.Poke()
			if len(failed) > 0 {
				return errors.New("could not update " + strings.Join(failed, ", "))
			}
			return nil
		},
		Finished: func(ctx context.Context, job store.Job) {
			a.auditJob(ctx, actor, job, audit.Entry{
				Action: "projects:env-apply", TargetType: "project", TargetName: p.Slug, ProjectSlug: p.Slug,
				Detail: map[string]any{"restart": restart},
			})
		},
	})
}

// revealProjectEnv returns secret shared values, audited with the keys only (SHV-5).
func (a *API) revealProjectEnv(w http.ResponseWriter, r *http.Request) error {
	var req RevealRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	if len(req.Keys) == 0 || len(req.Keys) > 100 {
		return validationError(map[string]string{"keys": "Ask for 1 to 100 keys."})
	}
	ctx := r.Context()
	p, err := a.db.ProjectBySlug(ctx, chi.URLParam(r, "slug"))
	if err != nil {
		return projectNotFound(err)
	}
	values, err := a.env.Values(ctx, p.ID)
	if err != nil {
		return err
	}
	out := RevealedValues{Values: map[string]string{}}
	for _, k := range req.Keys {
		if v, ok := values[k]; ok {
			out.Values[k] = v
		}
	}
	a.audit.Record(ctx, authz.FromContext(ctx), audit.Entry{
		Action: "projects:env-reveal", TargetType: "project", TargetName: p.Slug, ProjectSlug: p.Slug,
		Detail: map[string]any{"names": slices.Sorted(maps.Keys(out.Values))},
	})
	writeJSON(w, http.StatusOK, out)
	return nil
}

// envMove is one app's shared variable change after a membership change (SHV-2): leave the
// old project, join the new one, or both for a move.
type envMove struct {
	app       string
	from      *store.Project
	fromRows  map[string]string // provenance saved before a project delete; nil reads it
	to        *store.Project
	title     string
	projectID string
}

// submitEnvMove queues the job of one or more app moves, or returns nil when nothing has
// to be written: no managed variables to remove and no variables to apply.
func (a *API) submitEnvMoves(ctx context.Context, moves []envMove) (*store.Job, error) {
	var work []envMove
	for _, mv := range moves {
		needed := false
		if mv.from != nil {
			rows := mv.fromRows
			if rows == nil {
				var err error
				if rows, err = a.db.AppProvenance(ctx, mv.app, mv.from.ID); err != nil {
					return nil, err
				}
			}
			needed = len(rows) > 0
		}
		if mv.to != nil {
			vars, err := a.db.ProjectEnv(ctx, mv.to.ID)
			if err != nil {
				return nil, err
			}
			needed = needed || len(vars) > 0
		}
		if needed {
			work = append(work, mv)
		}
	}
	if len(work) == 0 {
		return nil, nil
	}
	if !a.snap.State().Server.Reachable {
		return nil, errDokkuUnreachable
	}
	steps := make([]string, 0, len(work))
	for _, mv := range work {
		steps = append(steps, "Update "+mv.app)
	}
	actor := authz.FromContext(ctx)
	job, err := a.jobs.Submit(ctx, jobs.Spec{
		Kind: envJobKind, Title: moves[0].title, ProjectID: moves[0].projectID, Actor: actor, Steps: steps,
		Run: func(ctx context.Context, t *jobs.Task) error {
			log := func(s string) { t.Write("stdout", s) }
			var failed []string
			for i, mv := range work {
				t.Step(i)
				if mv.from != nil {
					if _, err := a.env.Release(ctx, mv.from.ID, mv.app, mv.fromRows, log); err != nil {
						t.Write("stderr", mv.app+": "+err.Error())
						failed = append(failed, mv.app)
						continue
					}
				}
				if mv.to != nil {
					vars, err := a.env.Values(ctx, mv.to.ID)
					if err == nil {
						_, err = a.env.Apply(ctx, mv.to.ID, mv.app, vars, log)
					}
					if err != nil {
						t.Write("stderr", mv.app+": "+err.Error())
						failed = append(failed, mv.app)
					}
				}
			}
			a.snap.Poke()
			if len(failed) > 0 {
				return errors.New("could not update " + strings.Join(failed, ", "))
			}
			return nil
		},
	})
	if err != nil {
		return nil, err
	}
	return &job, nil
}

// writeMaybeJob answers 202 with the job, or 204 when no job was needed.
func writeMaybeJob(w http.ResponseWriter, job *store.Job) {
	if job == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, http.StatusAccepted, JobRef{JobId: job.ID})
}
