package api

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/go-chi/chi/v5"

	"gokku/internal/audit"
	"gokku/internal/authz"
	"gokku/internal/dokku"
	"gokku/internal/jobs"
	"gokku/internal/snapshot"
	"gokku/internal/store"
)

var errServiceNotFound = newError(http.StatusNotFound, ErrorCodeNotFound, "No service with this name on the server.")

// serviceTarget finds the service named in the path, in the snapshot.
func (a *API) serviceTarget(r *http.Request) (snapshot.Service, error) {
	typ, name := chi.URLParam(r, "type"), chi.URLParam(r, "name")
	if !dokku.IsDatastore(typ) || !dokku.ValidServiceName(name) {
		return snapshot.Service{}, errServiceNotFound
	}
	s, ok := a.snap.State().Service(typ, name)
	if !ok {
		return snapshot.Service{}, errServiceNotFound
	}
	return s, nil
}

// serviceProject returns the id and slug of the service's project, empty when unassigned.
func (a *API) serviceProject(ctx context.Context, s snapshot.Service) (string, string, error) {
	members, err := a.db.Memberships(ctx)
	if err != nil {
		return "", "", err
	}
	id := members.Services[store.ServiceKey{Type: s.Type, Name: s.Name}]
	if id == "" {
		return "", "", nil
	}
	p, err := a.db.ProjectByID(ctx, id)
	if err != nil {
		return "", "", err
	}
	return id, p.Slug, nil
}

// getService serves a service with masked connection details (SVC-4). The password never
// leaves the server here; revealService returns it, audited.
func (a *API) getService(w http.ResponseWriter, r *http.Request) error {
	s, err := a.serviceTarget(r)
	if err != nil {
		return err
	}
	v, err := a.loadView(r.Context())
	if err != nil {
		return err
	}
	sum := v.serviceSummary(s)
	out := ServiceDetail{Type: sum.Type, Name: sum.Name, Status: sum.Status, Version: sum.Version,
		Project: sum.Project, Links: sum.Links, LinkEnv: dokku.LinkEnvVar(s.Type)}
	if v.state.Server.Reachable {
		dsn, err := a.dokku.ServiceDSN(r.Context(), s.Type, s.Name)
		if err != nil {
			a.log.Warn("service connection", "type", s.Type, "service", s.Name, "error", err)
		} else {
			out.Connection = maskDSN(dsn)
		}
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// maskDSN splits a connection string into parts without its password.
func maskDSN(dsn string) *ServiceConnection {
	u, err := url.Parse(dsn)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil
	}
	c := &ServiceConnection{Scheme: u.Scheme, Host: u.Hostname()}
	if p := u.Port(); p != "" {
		c.Port = &p
	}
	if u.User != nil {
		if name := u.User.Username(); name != "" {
			c.Username = &name
		}
		_, has := u.User.Password()
		c.HasPassword = &has
	}
	if db := strings.TrimPrefix(u.Path, "/"); db != "" {
		c.Database = &db
	}
	c.Masked = dokku.RedactURLs(dsn)
	return c
}

// revealService returns the connection string, audited without it (SVC-4).
func (a *API) revealService(w http.ResponseWriter, r *http.Request) error {
	s, err := a.serviceTarget(r)
	if err != nil {
		return err
	}
	if !a.snap.State().Server.Reachable {
		return errDokkuUnreachable
	}
	ctx := r.Context()
	dsn, err := a.dokku.ServiceDSN(ctx, s.Type, s.Name)
	if err != nil {
		return err
	}
	_, slug, _ := a.serviceProject(ctx, s)
	a.audit.Record(ctx, authz.FromContext(ctx), audit.Entry{Action: "services:reveal", TargetType: "service",
		TargetName: s.Type + "/" + s.Name, ProjectSlug: slug})
	writeJSON(w, http.StatusOK, ServiceConnectionRevealed{Dsn: dsn})
	return nil
}

// createService creates a service in a project, as a job (SVC-2).
func (a *API) createService(w http.ResponseWriter, r *http.Request) error {
	var req CreateServiceRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	ctx := r.Context()
	typ, name := strings.TrimSpace(req.Type), strings.TrimSpace(req.Name)
	state := a.snap.State()
	fields := map[string]string{}
	if !dokku.IsDatastore(typ) {
		fields["type"] = "Choose a datastore."
	}
	if !dokku.ValidServiceName(name) {
		fields["name"] = "Use letters, digits, dashes and underscores, starting with a letter."
	}
	p, err := a.db.ProjectBySlug(ctx, req.Project)
	if err != nil {
		fields["project"] = "Choose a project."
	}
	if len(fields) > 0 {
		return validationError(fields)
	}
	if !state.Server.Reachable {
		return errDokkuUnreachable
	}
	if !slices.Contains(state.Server.Capabilities.Datastores, typ) {
		ae := newError(http.StatusBadRequest, ErrorCodePluginMissing, "The "+typ+" plugin is not installed or not usable.")
		ae.Fields = map[string]string{"type": "This datastore's plugin is not installed."}
		return ae
	}
	if _, exists := state.Service(typ, name); exists {
		ae := newError(http.StatusConflict, ErrorCodeServiceExists, "A "+typ+" service with this name exists already.")
		ae.Fields = map[string]string{"name": "This name is taken."}
		return ae
	}
	actor := authz.FromContext(ctx)
	key := store.ServiceKey{Type: typ, Name: name}
	job, err := a.jobs.Submit(ctx, jobs.Spec{
		Kind: "service.create", Title: "Create " + typ + " service " + name, ProjectID: p.ID, Actor: actor,
		Steps: []string{"Create"},
		Run: func(ctx context.Context, t *jobs.Task) error {
			if err := a.dokku.CreateService(ctx, typ, name, t.Line); err != nil {
				return err
			}
			if err := a.db.AssignService(ctx, key, p.ID); err != nil {
				return err
			}
			_ = a.snap.Refresh(ctx)
			a.projectChanged(p.Slug)
			return nil
		},
		Finished: func(ctx context.Context, job store.Job) {
			a.auditJob(ctx, actor, job, audit.Entry{Action: "services:create", TargetType: "service",
				TargetName: typ + "/" + name, ProjectSlug: p.Slug})
		},
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusAccepted, JobRef{JobId: job.ID})
	return nil
}

// destroyService destroys a service with no linked apps, as a job (SVC-2).
func (a *API) destroyService(w http.ResponseWriter, r *http.Request) error {
	s, err := a.serviceTarget(r)
	if err != nil {
		return err
	}
	if r.URL.Query().Get("confirm") != s.Name {
		ae := newError(http.StatusBadRequest, ErrorCodeConfirmMismatch, "Type the service's name to confirm.")
		ae.Fields = map[string]string{"confirm": "Type " + s.Name + " to confirm."}
		return ae
	}
	if len(s.Links) > 0 {
		return newError(http.StatusConflict, ErrorCodeServiceLinked,
			"Unlink "+strings.Join(s.Links, ", ")+" first. Dokku does not destroy a linked service.")
	}
	if !a.snap.State().Server.Reachable {
		return errDokkuUnreachable
	}
	ctx := r.Context()
	projectID, slug, err := a.serviceProject(ctx, s)
	if err != nil {
		return err
	}
	actor := authz.FromContext(ctx)
	typ, name := s.Type, s.Name
	job, err := a.jobs.Submit(ctx, jobs.Spec{
		Kind: "service.destroy", Title: "Destroy " + typ + " service " + name, ProjectID: projectID, Actor: actor,
		Steps: []string{"Destroy"},
		Run: func(ctx context.Context, t *jobs.Task) error {
			if err := a.dokku.DestroyService(ctx, typ, name, t.Line); err != nil {
				return err
			}
			if err := a.db.ForgetService(ctx, store.ServiceKey{Type: typ, Name: name}); err != nil {
				return err
			}
			_ = a.snap.Refresh(ctx)
			if slug != "" {
				a.projectChanged(slug)
			}
			return nil
		},
		Finished: func(ctx context.Context, job store.Job) {
			a.auditJob(ctx, actor, job, audit.Entry{Action: "services:destroy", TargetType: "service",
				TargetName: typ + "/" + name, ProjectSlug: slug})
		},
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusAccepted, JobRef{JobId: job.ID})
	return nil
}

// serviceLink links or unlinks an app (SVC-3). Both must be in the same project, or both
// unassigned. The job holds the app's lock, since the app restarts.
func (a *API) serviceLink(link bool) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		s, err := a.serviceTarget(r)
		if err != nil {
			return err
		}
		var req ServiceLinkRequest
		if err := decode(r, &req); err != nil {
			return err
		}
		state := a.snap.State()
		app, ok := state.App(strings.TrimSpace(req.App))
		if !ok {
			return validationError(map[string]string{"app": "Choose an app on the server."})
		}
		linked := slices.Contains(s.Links, app.Name)
		if link && linked {
			return newError(http.StatusConflict, ErrorCodeAlreadyLinked, app.Name+" is linked to "+s.Name+" already.")
		}
		if !link && !linked {
			return newError(http.StatusConflict, ErrorCodeNotLinked, app.Name+" is not linked to "+s.Name+".")
		}
		ctx := r.Context()
		members, err := a.db.Memberships(ctx)
		if err != nil {
			return err
		}
		serviceProject := members.Services[store.ServiceKey{Type: s.Type, Name: s.Name}]
		appProject := members.Apps[app.Name]
		if link && serviceProject != appProject {
			return newError(http.StatusConflict, ErrorCodeCrossProjectLink,
				"Link only an app and a service of the same project. Move one of them first.")
		}
		if !state.Server.Reachable {
			return errDokkuUnreachable
		}
		_, slug, _ := a.appProject(ctx, app.Name)
		actor := authz.FromContext(ctx)
		typ, name, appName := s.Type, s.Name, app.Name
		verb, action, run := "Link", "services:link", a.dokku.LinkService
		if !link {
			verb, action, run = "Unlink", "services:unlink", a.dokku.UnlinkService
		}
		job, err := a.jobs.Submit(ctx, jobs.Spec{
			Kind: "service." + strings.ToLower(verb), Title: verb + " " + name + " and " + appName,
			App: appName, ProjectID: appProject, Actor: actor, Steps: []string{verb},
			Run: func(ctx context.Context, t *jobs.Task) error {
				if err := run(ctx, typ, name, appName, t.Line); err != nil {
					return err
				}
				_ = a.snap.Refresh(ctx)
				return nil
			},
			Finished: func(ctx context.Context, job store.Job) {
				a.auditJob(ctx, actor, job, audit.Entry{Action: action, TargetType: "service",
					TargetName: typ + "/" + name, ProjectSlug: slug, Detail: map[string]any{"app": appName}})
			},
		})
		if err != nil {
			return err
		}
		writeJSON(w, http.StatusAccepted, JobRef{JobId: job.ID})
		return nil
	}
}

// serviceAction starts, stops or restarts a service container, as a job (SVC-2).
func (a *API) serviceAction(action string) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		s, err := a.serviceTarget(r)
		if err != nil {
			return err
		}
		if !a.snap.State().Server.Reachable {
			return errDokkuUnreachable
		}
		ctx := r.Context()
		projectID, slug, err := a.serviceProject(ctx, s)
		if err != nil {
			return err
		}
		actor := authz.FromContext(ctx)
		typ, name := s.Type, s.Name
		verb := strings.ToUpper(action[:1]) + action[1:]
		job, err := a.jobs.Submit(ctx, jobs.Spec{
			Kind: "service." + action, Title: verb + " " + typ + " service " + name, ProjectID: projectID, Actor: actor,
			Steps: []string{verb},
			Run: func(ctx context.Context, t *jobs.Task) error {
				if err := a.dokku.ServiceLifecycle(ctx, action, typ, name, t.Line); err != nil {
					return err
				}
				_ = a.snap.Refresh(ctx)
				return nil
			},
			Finished: func(ctx context.Context, job store.Job) {
				a.auditJob(ctx, actor, job, audit.Entry{Action: "services:" + action, TargetType: "service",
					TargetName: typ + "/" + name, ProjectSlug: slug})
			},
		})
		if err != nil {
			return err
		}
		writeJSON(w, http.StatusAccepted, JobRef{JobId: job.ID})
		return nil
	}
}
