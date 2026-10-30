package api

import (
	"context"
	"time"

	"gokku/internal/snapshot"
	"gokku/internal/store"
)

// view joins the Dokku snapshot with Gokku's projects and membership for one request.
// Membership rows that name an app or service Dokku no longer has are ignored.
type view struct {
	state    *snapshot.State
	projects map[string]store.Project // by id
	members  store.Membership
}

func (a *API) loadView(ctx context.Context) (*view, error) {
	projects, err := a.db.Projects(ctx)
	if err != nil {
		return nil, err
	}
	members, err := a.db.Memberships(ctx)
	if err != nil {
		return nil, err
	}
	v := &view{state: a.snap.State(), projects: make(map[string]store.Project, len(projects)), members: members}
	for _, p := range projects {
		v.projects[p.ID] = p
	}
	return v, nil
}

func (v *view) projectRef(id string) *ProjectRef {
	p, ok := v.projects[id]
	if !ok {
		return nil
	}
	return &ProjectRef{Slug: p.Slug, Name: p.Name}
}

func (v *view) appSummary(app snapshot.App) AppSummary {
	services := make([]ServiceRef, 0, len(app.Services))
	for _, s := range app.Services {
		services = append(services, ServiceRef{Type: s.Type, Name: s.Name})
	}
	out := AppSummary{
		Name:         app.Name,
		Status:       AppStatus(app.Status),
		Project:      v.projectRef(v.members.Apps[app.Name]),
		Domains:      nonNil(app.Domains),
		DeploySource: app.DeploySource,
		Services:     services,
		DeployedAt:   timePtr(app.DeployedAt),
	}
	if app.Revision != "" {
		out.Revision = &app.Revision
	}
	return out
}

func (v *view) appDetail(app snapshot.App) AppDetail {
	s := v.appSummary(app)
	processes := make([]Process, 0, len(app.Processes))
	for _, p := range app.Processes {
		processes = append(processes, Process{Type: p.Type, Count: p.Count, Running: p.Running})
	}
	out := AppDetail{
		Name: s.Name, Status: s.Status, Project: s.Project, Domains: s.Domains, Revision: s.Revision,
		DeployedAt: s.DeployedAt, DeploySource: s.DeploySource, Services: s.Services,
		Deployed:  app.Deployed,
		Locked:    app.Locked,
		CreatedAt: timePtr(app.CreatedAt),
		Builder:   app.Builder,
		Processes: processes,
	}
	if app.SourceImage != "" {
		out.SourceImage = &app.SourceImage
	}
	return out
}

func (v *view) serviceSummary(s snapshot.Service) ServiceSummary {
	return ServiceSummary{
		Type:    s.Type,
		Name:    s.Name,
		Status:  serviceStatus(s.Status),
		Version: s.Version,
		Project: v.projectRef(v.members.Services[store.ServiceKey{Type: s.Type, Name: s.Name}]),
		Links:   nonNil(s.Links),
	}
}

// serviceStatus maps the docker state of a service container to the API status.
func serviceStatus(state string) ServiceStatus {
	switch state {
	case "running":
		return ServiceStatusRunning
	case "", "missing":
		return ServiceStatusMissing
	default:
		return ServiceStatusStopped
	}
}

// projectApps returns the snapshot apps that belong to the project.
func (v *view) projectApps(projectID string) []snapshot.App {
	var out []snapshot.App
	for _, app := range v.state.Apps {
		if v.members.Apps[app.Name] == projectID {
			out = append(out, app)
		}
	}
	return out
}

// projectServices returns the snapshot services that belong to the project.
func (v *view) projectServices(projectID string) []snapshot.Service {
	var out []snapshot.Service
	for _, s := range v.state.Services {
		if v.members.Services[store.ServiceKey{Type: s.Type, Name: s.Name}] == projectID {
			out = append(out, s)
		}
	}
	return out
}

func statusCounts(apps []snapshot.App) (StatusCounts, AppStatus) {
	var c StatusCounts
	statuses := make([]snapshot.Status, 0, len(apps))
	for _, a := range apps {
		statuses = append(statuses, a.Status)
		switch a.Status {
		case snapshot.StatusRunning:
			c.Running++
		case snapshot.StatusStopped:
			c.Stopped++
		case snapshot.StatusDeploying:
			c.Deploying++
		case snapshot.StatusFailed:
			c.Failed++
		}
	}
	return c, AppStatus(snapshot.Health(statuses))
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// nonNil returns an empty slice for nil, so JSON carries [] rather than null.
func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
