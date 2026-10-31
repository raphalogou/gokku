package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"gokku/internal/audit"
	"gokku/internal/authz"
	"gokku/internal/dokku"
	"gokku/internal/jobs"
	"gokku/internal/store"
)

var errAppProtected = newError(http.StatusConflict, ErrorCodeAppProtected,
	"This app is protected. Remove it from the protected apps in Settings first.")

func (a *API) refuseProtected(ctx context.Context, app string) error {
	protected, err := a.protectedApps(ctx)
	if err != nil {
		return err
	}
	if protected[app] {
		return errAppProtected
	}
	return nil
}

// gitRemote is what `git remote add dokku` takes for the app (APP-3).
func (a *API) gitRemote(r *http.Request, app string) string {
	host := a.dokkuHost
	if host == "" {
		host = requestHost(r)
	}
	return "dokku@" + host + ":" + app
}

// createApp implements APP-3: create the app in Dokku, put it in its project and apply the
// project's shared variables (SHV-2).
func (a *API) createApp(w http.ResponseWriter, r *http.Request) error {
	var req CreateAppRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	ctx := r.Context()
	name := strings.TrimSpace(req.Name)
	fields := map[string]string{}
	if !dokku.ValidAppName(name) {
		fields["name"] = "Use lowercase letters, digits, dots and dashes, starting with a letter or digit."
	}
	switch req.Source {
	case AppSourceGit, AppSourceImage, AppSourceRepository:
	default:
		fields["source"] = "Choose git, image or repository."
	}
	p, err := a.db.ProjectBySlug(ctx, req.Project)
	if err != nil {
		fields["project"] = "Choose a project."
	}
	if len(fields) > 0 {
		return validationError(fields)
	}
	state := a.snap.State()
	if !state.Server.Reachable {
		return errDokkuUnreachable
	}
	if _, exists := state.App(name); exists {
		ae := newError(http.StatusConflict, ErrorCodeAppExists, "An app with this name exists already.")
		ae.Fields = map[string]string{"name": "This name is taken."}
		return ae
	}

	actor := authz.FromContext(ctx)
	entry := audit.Entry{Action: "apps:create", TargetType: "app", TargetName: name, ProjectSlug: p.Slug,
		Detail: map[string]any{"source": req.Source}}
	if err := a.dokku.CreateApp(ctx, name); err != nil {
		entry.Result = audit.ResultFail
		a.audit.Record(ctx, actor, entry)
		return err
	}
	if err := a.db.AssignApp(ctx, name, p.ID); err != nil {
		return err
	}
	job, err := a.submitEnvMoves(ctx, []envMove{{app: name, to: &p, title: "Update shared variables of " + name, projectID: p.ID}})
	if err != nil {
		return err
	}
	if job != nil {
		entry.JobID = job.ID
	}
	a.audit.Record(ctx, actor, entry)
	_ = a.snap.Refresh(ctx)
	a.projectChanged(p.Slug)

	v, err := a.loadView(ctx)
	if err != nil {
		return err
	}
	out := AppCreated{GitRemote: a.gitRemote(r, name), Source: req.Source,
		App: AppSummary{Name: name, Status: AppStatusStopped, Domains: []string{}, Services: []ServiceRef{},
			Project: &ProjectRef{Slug: p.Slug, Name: p.Name}}}
	if app, ok := v.state.App(name); ok {
		out.App = v.appSummary(app)
	}
	if job != nil {
		out.JobId = &job.ID
	}
	writeJSON(w, http.StatusCreated, out)
	return nil
}

// updateApp renames an app (APP-4) or moves it to another project (APP-7). Both are admin
// actions and are refused for protected apps (SET-2).
func (a *API) updateApp(w http.ResponseWriter, r *http.Request) error {
	var req UpdateAppRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	ctx := r.Context()
	if (req.Name == nil) == (req.Project == nil) {
		return validationError(map[string]string{"name": "Send either a new name or a project."})
	}
	if req.Project != nil && !authz.Can(authz.FromContext(ctx), authz.AppMove, authz.Resource{}) {
		return errForbidden
	}
	state := a.snap.State()
	app, ok := state.App(chi.URLParam(r, "name"))
	if !ok {
		return newError(http.StatusNotFound, ErrorCodeNotFound, "No app with this name on the server.")
	}
	if !state.Server.Reachable {
		return errDokkuUnreachable
	}
	if err := a.refuseProtected(ctx, app.Name); err != nil {
		return err
	}
	if req.Name != nil {
		return a.renameApp(w, r, app.Name, strings.TrimSpace(*req.Name))
	}
	return a.moveApp(w, r, app.Name, *req.Project)
}

func (a *API) renameApp(w http.ResponseWriter, r *http.Request, from, to string) error {
	ctx := r.Context()
	if !dokku.ValidAppName(to) {
		return validationError(map[string]string{"name": "Use lowercase letters, digits, dots and dashes, starting with a letter or digit."})
	}
	if to == from {
		return validationError(map[string]string{"name": "This is the current name."})
	}
	if _, exists := a.snap.State().App(to); exists {
		ae := newError(http.StatusConflict, ErrorCodeAppExists, "An app with this name exists already.")
		ae.Fields = map[string]string{"name": "This name is taken."}
		return ae
	}
	projectID, slug, _ := a.appProject(ctx, from)
	actor := authz.FromContext(ctx)
	job, err := a.jobs.Submit(ctx, jobs.Spec{
		Kind: "app.rename", Title: "Rename " + from + " to " + to, App: from, ProjectID: projectID, Actor: actor,
		Steps: []string{"Rename and redeploy"},
		Run: func(ctx context.Context, t *jobs.Task) error {
			if err := a.dokku.RenameApp(ctx, from, to, t.Line); err != nil {
				return err
			}
			// The membership, provenance and history follow the new name (APP-4).
			if err := a.db.RenameApp(ctx, from, to); err != nil {
				return err
			}
			a.snap.Poke()
			return nil
		},
		Finished: func(ctx context.Context, job store.Job) {
			a.auditJob(ctx, actor, job, audit.Entry{Action: "apps:rename", TargetType: "app", TargetName: from,
				ProjectSlug: slug, Detail: map[string]any{"to": to}})
		},
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusAccepted, JobRef{JobId: job.ID})
	return nil
}

func (a *API) moveApp(w http.ResponseWriter, r *http.Request, app, slug string) error {
	ctx := r.Context()
	to, err := a.db.ProjectBySlug(ctx, slug)
	if err != nil {
		return validationError(map[string]string{"project": "Choose a project."})
	}
	members, err := a.db.Memberships(ctx)
	if err != nil {
		return err
	}
	var from *store.Project
	if id := members.Apps[app]; id != "" {
		if id == to.ID {
			return validationError(map[string]string{"project": "The app is in this project already."})
		}
		if p, err := a.db.ProjectByID(ctx, id); err == nil {
			from = &p
		}
	}
	if err := a.db.AssignApp(ctx, app, to.ID); err != nil {
		return err
	}
	job, err := a.submitEnvMoves(ctx, []envMove{{app: app, from: from, to: &to, title: "Update shared variables of " + app, projectID: to.ID}})
	if err != nil {
		return err
	}
	detail := map[string]any{"to": to.Slug}
	if from != nil {
		detail["from"] = from.Slug
		a.projectChanged(from.Slug)
	}
	entry := audit.Entry{Action: "apps:move", TargetType: "app", TargetName: app, ProjectSlug: to.Slug, Detail: detail}
	if job != nil {
		entry.JobID = job.ID
	}
	a.audit.Record(ctx, authz.FromContext(ctx), entry)
	a.projectChanged(to.Slug)
	writeMaybeJob(w, job)
	return nil
}

// destroyApp implements APP-5: the caller confirms with the app name; linked services are
// kept; the membership and provenance rows go with the app.
func (a *API) destroyApp(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	state := a.snap.State()
	app, ok := state.App(chi.URLParam(r, "name"))
	if !ok {
		return newError(http.StatusNotFound, ErrorCodeNotFound, "No app with this name on the server.")
	}
	if r.URL.Query().Get("confirm") != app.Name {
		ae := newError(http.StatusBadRequest, ErrorCodeConfirmMismatch, "Type the app's name to confirm.")
		ae.Fields = map[string]string{"confirm": "Type " + app.Name + " to confirm."}
		return ae
	}
	if !state.Server.Reachable {
		return errDokkuUnreachable
	}
	if err := a.refuseProtected(ctx, app.Name); err != nil {
		return err
	}
	projectID, slug, _ := a.appProject(ctx, app.Name)
	actor := authz.FromContext(ctx)
	name := app.Name
	job, err := a.jobs.Submit(ctx, jobs.Spec{
		Kind: "app.destroy", Title: "Destroy " + name, App: name, ProjectID: projectID, Actor: actor,
		Steps: []string{"Destroy"},
		Run: func(ctx context.Context, t *jobs.Task) error {
			if err := a.dokku.DestroyApp(ctx, name, t.Line); err != nil {
				return err
			}
			if err := a.db.ForgetApp(ctx, name); err != nil {
				return err
			}
			a.snap.Poke()
			if slug != "" {
				a.projectChanged(slug)
			}
			return nil
		},
		Finished: func(ctx context.Context, job store.Job) {
			a.auditJob(ctx, actor, job, audit.Entry{Action: "apps:destroy", TargetType: "app", TargetName: name, ProjectSlug: slug})
		},
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusAccepted, JobRef{JobId: job.ID})
	return nil
}
