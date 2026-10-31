package api

import (
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"gokku/internal/audit"
	"gokku/internal/authz"
	"gokku/internal/dokku"
	"gokku/internal/events"
	"gokku/internal/store"
)

// projectActivityLimit is how many audit entries the project overview shows (PRJ-3).
const projectActivityLimit = 10

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)

// listProjects serves the Projects screen: every project with its apps' statuses.
func (a *API) listProjects(w http.ResponseWriter, r *http.Request) error {
	v, err := a.loadView(r.Context())
	if err != nil {
		return err
	}
	projects, err := a.db.Projects(r.Context())
	if err != nil {
		return err
	}
	out := make([]ProjectSummary, 0, len(projects))
	for _, p := range projects {
		apps := v.projectApps(p.ID)
		counts, health := statusCounts(apps)
		states := make([]AppState, 0, len(apps))
		for _, app := range apps {
			states = append(states, AppState{Name: app.Name, Status: AppStatus(app.Status)})
		}
		services := []ServiceRef{}
		for _, s := range v.projectServices(p.ID) {
			services = append(services, ServiceRef{Type: s.Type, Name: s.Name})
		}
		out = append(out, ProjectSummary{
			Slug: p.Slug, Name: p.Name, Description: p.Description,
			Health: health, StatusCounts: counts, Apps: states, Services: services,
		})
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (a *API) getProject(w http.ResponseWriter, r *http.Request) error {
	p, err := a.db.ProjectBySlug(r.Context(), chi.URLParam(r, "slug"))
	if err != nil {
		return projectNotFound(err)
	}
	detail, err := a.projectDetail(r, p)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, detail)
	return nil
}

// projectDetail builds the project overview (PRJ-3).
func (a *API) projectDetail(r *http.Request, p store.Project) (ProjectDetail, error) {
	v, err := a.loadView(r.Context())
	if err != nil {
		return ProjectDetail{}, err
	}
	entries, err := a.db.ProjectAuditEntries(r.Context(), p.Slug, projectActivityLimit)
	if err != nil {
		return ProjectDetail{}, err
	}
	apps := v.projectApps(p.ID)
	counts, health := statusCounts(apps)
	out := ProjectDetail{
		Slug: p.Slug, Name: p.Name, Description: p.Description, CreatedAt: p.CreatedAt.Time,
		Health: health, StatusCounts: counts,
		Apps: make([]AppSummary, 0, len(apps)), Services: []ServiceSummary{},
		Activity: make([]ActivityEntry, 0, len(entries)),
	}
	var lastDeploy time.Time
	for _, app := range apps {
		out.Apps = append(out.Apps, v.appSummary(app))
		out.DomainCount += len(app.Domains)
		if app.DeployedAt.After(lastDeploy) {
			lastDeploy = app.DeployedAt
		}
	}
	out.LastDeployAt = timePtr(lastDeploy)
	for _, s := range v.projectServices(p.ID) {
		out.Services = append(out.Services, v.serviceSummary(s))
	}
	for _, e := range entries {
		out.Activity = append(out.Activity, toActivity(e))
	}
	return out, nil
}

func toActivity(e store.AuditEntry) ActivityEntry {
	return ActivityEntry{
		Id: e.ID, Ts: e.TS.Time, ActorName: e.ActorName, ActorType: ActivityEntryActorType(e.ActorType),
		Action: e.Action, TargetType: e.TargetType, TargetName: e.TargetName, ProjectSlug: e.ProjectSlug,
		JobId: e.JobID, Result: ActivityEntryResult(e.Result), Source: ActivityEntrySource(e.Source),
	}
}

// createProject implements PRJ-1.
func (a *API) createProject(w http.ResponseWriter, r *http.Request) error {
	var req CreateProjectRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	ctx := r.Context()
	p := store.Project{ID: store.NewID(), Slug: strings.TrimSpace(req.Slug), Name: strings.TrimSpace(req.Name)}
	if req.Description != nil {
		p.Description = strings.TrimSpace(*req.Description)
	}
	if err := validateProject(p.Name, p.Description, &p.Slug); err != nil {
		return err
	}
	principal := authz.FromContext(ctx)
	p.CreatedBy = &principal.UserID
	if err := a.db.CreateProject(ctx, p); err != nil {
		if errors.Is(err, store.ErrConflict) {
			ae := newError(http.StatusConflict, ErrorCodeProjectExists, "A project with this URL name already exists.")
			ae.Fields = map[string]string{"slug": "This URL name is taken."}
			return ae
		}
		return err
	}
	a.audit.Record(ctx, principal, audit.Entry{
		Action: "projects:create", TargetType: "project", TargetName: p.Slug, ProjectSlug: p.Slug,
		Detail: map[string]any{"name": p.Name},
	})
	a.projectChanged(p.Slug)
	created, err := a.db.ProjectBySlug(ctx, p.Slug)
	if err != nil {
		return err
	}
	detail, err := a.projectDetail(r, created)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, detail)
	return nil
}

// updateProject changes the name and description. The slug is immutable (PRJ-1).
func (a *API) updateProject(w http.ResponseWriter, r *http.Request) error {
	var req UpdateProjectRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	ctx := r.Context()
	p, err := a.db.ProjectBySlug(ctx, chi.URLParam(r, "slug"))
	if err != nil {
		return projectNotFound(err)
	}
	name, description := p.Name, p.Description
	if req.Name != nil {
		name = strings.TrimSpace(*req.Name)
	}
	if req.Description != nil {
		description = strings.TrimSpace(*req.Description)
	}
	if err := validateProject(name, description, nil); err != nil {
		return err
	}
	detail := map[string]any{}
	if name != p.Name {
		detail["name"] = map[string]any{"from": p.Name, "to": name}
	}
	if description != p.Description {
		detail["description"] = "changed"
	}
	if len(detail) > 0 {
		if err := a.db.UpdateProject(ctx, p.ID, name, description); err != nil {
			return err
		}
		a.audit.Record(ctx, authz.FromContext(ctx), audit.Entry{
			Action: "projects:update", TargetType: "project", TargetName: p.Slug, ProjectSlug: p.Slug, Detail: detail,
		})
		a.projectChanged(p.Slug)
		p.Name, p.Description = name, description
	}
	out, err := a.projectDetail(r, p)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// deleteProject releases the project's apps and services and deletes it. The caller
// confirms with the slug (PRJ-4).
//
// Shared variables arrive in M4; deleting a project will then also remove its managed
// variables from member apps (SHV-2).
func (a *API) deleteProject(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	p, err := a.db.ProjectBySlug(ctx, chi.URLParam(r, "slug"))
	if err != nil {
		return projectNotFound(err)
	}
	if r.URL.Query().Get("confirm") != p.Slug {
		ae := newError(http.StatusBadRequest, ErrorCodeConfirmMismatch, "Type the project's URL name to confirm.")
		ae.Fields = map[string]string{"confirm": "Type " + p.Slug + " to confirm."}
		return ae
	}
	if err := a.db.DeleteProject(ctx, p.ID); err != nil {
		return err
	}
	a.audit.Record(ctx, authz.FromContext(ctx), audit.Entry{
		Action: "projects:delete", TargetType: "project", TargetName: p.Slug, ProjectSlug: p.Slug,
		Detail: map[string]any{"name": p.Name},
	})
	a.projectChanged(p.Slug)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// addProjectApp puts an app in the project, moving it out of another one (PRJ-2).
func (a *API) addProjectApp(w http.ResponseWriter, r *http.Request) error {
	return a.changeMembership(w, r, "app", true)
}

func (a *API) removeProjectApp(w http.ResponseWriter, r *http.Request) error {
	return a.changeMembership(w, r, "app", false)
}

func (a *API) addProjectService(w http.ResponseWriter, r *http.Request) error {
	return a.changeMembership(w, r, "service", true)
}

func (a *API) removeProjectService(w http.ResponseWriter, r *http.Request) error {
	return a.changeMembership(w, r, "service", false)
}

// changeMembership adds or removes an app or a service. The object must exist in Dokku
// to be added; removing works on a name Dokku no longer knows, to clean up.
func (a *API) changeMembership(w http.ResponseWriter, r *http.Request, kind string, add bool) error {
	ctx := r.Context()
	p, err := a.db.ProjectBySlug(ctx, chi.URLParam(r, "slug"))
	if err != nil {
		return projectNotFound(err)
	}
	state := a.snap.State()
	var target string
	var svc store.ServiceKey
	if kind == "app" {
		target = chi.URLParam(r, "app")
		_, exists := state.App(target)
		if !dokku.ValidAppName(target) || (add && !exists) {
			return newError(http.StatusNotFound, ErrorCodeNotFound, "No app with this name on the server.")
		}
	} else {
		svc = store.ServiceKey{Type: chi.URLParam(r, "type"), Name: chi.URLParam(r, "name")}
		target = svc.Type + "/" + svc.Name
		_, exists := state.Service(svc.Type, svc.Name)
		if !dokku.IsDatastore(svc.Type) || !dokku.ValidServiceName(svc.Name) || (add && !exists) {
			return newError(http.StatusNotFound, ErrorCodeNotFound, "No service with this name on the server.")
		}
	}

	var from string
	action := "projects:remove-" + kind
	if add {
		action = "projects:add-" + kind
		if members, err := a.db.Memberships(ctx); err == nil {
			from = members.Apps[target]
			if kind == "service" {
				from = members.Services[svc]
			}
		}
	}
	switch {
	case kind == "app" && add:
		err = a.db.AssignApp(ctx, target, p.ID)
	case kind == "app":
		err = a.db.ReleaseApp(ctx, target, p.ID)
	case add:
		err = a.db.AssignService(ctx, svc, p.ID)
	default:
		err = a.db.ReleaseService(ctx, svc, p.ID)
	}
	if errors.Is(err, store.ErrNotFound) {
		return newError(http.StatusNotFound, ErrorCodeNotFound, "This "+kind+" is not in the project.")
	}
	if err != nil {
		return err
	}

	detail := map[string]any{}
	if from != "" && from != p.ID {
		if old, err := a.db.ProjectByID(ctx, from); err == nil {
			detail["moved_from"] = old.Slug
			a.projectChanged(old.Slug)
		}
	}
	if from != p.ID || !add {
		a.audit.Record(ctx, authz.FromContext(ctx), audit.Entry{
			Action: action, TargetType: kind, TargetName: target, ProjectSlug: p.Slug, Detail: detail,
		})
		a.projectChanged(p.Slug)
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// projectChanged tells event subscribers to refetch project data.
func (a *API) projectChanged(slug string) {
	a.hub.Publish(events.Event{Type: events.ProjectUpdated, Data: map[string]string{"slug": slug}})
}

func validateProject(name, description string, slug *string) error {
	fields := map[string]string{}
	if n := len([]rune(name)); n < 1 || n > 80 {
		fields["name"] = "Use 1 to 80 characters."
	}
	if len([]rune(description)) > 500 {
		fields["description"] = "Use at most 500 characters."
	}
	if slug != nil && !slugRe.MatchString(*slug) {
		fields["slug"] = "Use up to 40 lowercase letters, digits and dashes, starting with a letter or digit."
	}
	if len(fields) > 0 {
		return validationError(fields)
	}
	return nil
}

func projectNotFound(err error) error {
	if errors.Is(err, store.ErrNotFound) {
		return newError(http.StatusNotFound, ErrorCodeNotFound, "No project with this URL name.")
	}
	return err
}
