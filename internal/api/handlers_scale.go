package api

import (
	"context"
	"maps"
	"net/http"
	"slices"

	"gokku/internal/audit"
	"gokku/internal/authz"
	"gokku/internal/dokku"
	"gokku/internal/jobs"
	"gokku/internal/snapshot"
	"gokku/internal/store"
)

func (a *API) appScale(ctx context.Context, app snapshot.App) (AppScale, error) {
	scale, err := a.dokku.Scale(ctx, app.Name)
	if err != nil {
		return AppScale{}, err
	}
	running := map[string]int{}
	for _, p := range app.Processes {
		running[p.Type] = p.Running
		if _, ok := scale[p.Type]; !ok {
			scale[p.Type] = p.Count
		}
	}
	out := AppScale{CanScale: app.CanScale, Processes: make([]ProcessScale, 0, len(scale))}
	for _, typ := range slices.Sorted(maps.Keys(scale)) {
		out.Processes = append(out.Processes, ProcessScale{Type: typ, Quantity: scale[typ], Running: running[typ]})
	}
	return out, nil
}

func (a *API) getAppScale(w http.ResponseWriter, r *http.Request) error {
	app, err := a.configTarget(r)
	if err != nil {
		return err
	}
	out, err := a.appScale(r.Context(), app)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// putAppScale applies new instance counts with ps:scale as a job (SCL-1).
func (a *API) putAppScale(w http.ResponseWriter, r *http.Request) error {
	var req ScaleRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	app, err := a.configTarget(r)
	if err != nil {
		return err
	}
	if !app.CanScale {
		return validationError(map[string]string{"processes": "The app's app.json sets the formation; change it there."})
	}
	ctx := r.Context()
	current, err := a.appScale(ctx, app)
	if err != nil {
		return err
	}
	known := map[string]int{}
	for _, p := range current.Processes {
		known[p.Type] = p.Quantity
	}
	fields := map[string]string{}
	changes := map[string]int{}
	for typ, n := range req.Processes {
		old, ok := known[typ]
		switch {
		case !ok:
			fields[typ] = "The app has no such process type."
		case n < 0 || n > dokku.MaxScale:
			fields[typ] = "Use 0 to 100 instances."
		case n != old:
			changes[typ] = n
		}
	}
	if len(fields) > 0 {
		return validationError(fields)
	}
	if len(changes) == 0 {
		return validationError(map[string]string{"processes": "Nothing to change."})
	}
	projectID, slug, _ := a.appProject(ctx, app.Name)
	actor := authz.FromContext(ctx)
	detail := map[string]any{}
	for typ, n := range changes {
		detail[typ] = map[string]any{"from": known[typ], "to": n}
	}
	job, err := a.jobs.Submit(ctx, jobs.Spec{
		Kind: "app.scale", Title: "Scale " + app.Name, App: app.Name, ProjectID: projectID, Actor: actor,
		Steps: []string{"Scale and restart"},
		Run: func(ctx context.Context, t *jobs.Task) error {
			return a.dokku.SetScale(ctx, app.Name, changes, t.Line)
		},
		Finished: func(ctx context.Context, job store.Job) {
			a.auditJob(ctx, actor, job, audit.Entry{Action: "apps:scale", TargetType: "app", TargetName: app.Name,
				ProjectSlug: slug, Detail: detail})
		},
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusAccepted, JobRef{JobId: job.ID})
	return nil
}
