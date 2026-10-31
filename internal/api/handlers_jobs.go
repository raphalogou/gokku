package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"gokku/internal/audit"
	"gokku/internal/authz"
	"gokku/internal/dokku"
	"gokku/internal/jobs"
	"gokku/internal/store"
)

// lifecycleKinds maps a lifecycle action to its job kind and step name.
var lifecycleKinds = map[string]struct{ kind, step string }{
	dokku.ActionRestart: {store.JobKindRestart, "Restart"},
	dokku.ActionStop:    {store.JobKindStop, "Stop"},
	dokku.ActionStart:   {store.JobKindStart, "Start"},
	dokku.ActionRebuild: {store.JobKindRebuild, "Rebuild"},
}

// protectedActions are refused for protected apps (SET-2).
var protectedActions = []string{dokku.ActionStop, dokku.ActionRebuild}

// appAction returns the handler of POST /apps/{name}/actions/<action> (APP-6): it queues a
// job and answers 202 with its id.
func (a *API) appAction(action string) HandlerFunc {
	k := lifecycleKinds[action]
	return func(w http.ResponseWriter, r *http.Request) error {
		ctx := r.Context()
		state := a.snap.State()
		name := chi.URLParam(r, "name")
		if _, ok := state.App(name); !ok {
			return newError(http.StatusNotFound, ErrorCodeNotFound, "No app with this name on the server.")
		}
		if !state.Server.Reachable {
			return errDokkuUnreachable
		}
		if slices.Contains(protectedActions, action) {
			protected, err := a.protectedApps(ctx)
			if err != nil {
				return err
			}
			if protected[name] {
				return newError(http.StatusConflict, ErrorCodeAppProtected,
					"This app is protected. Remove it from the protected apps in Settings first.")
			}
		}

		members, err := a.db.Memberships(ctx)
		if err != nil {
			return err
		}
		projectID := members.Apps[name]
		var projectSlug string
		if projectID != "" {
			if p, err := a.db.ProjectByID(ctx, projectID); err == nil {
				projectSlug = p.Slug
			}
		}
		actor := authz.FromContext(ctx)
		job, err := a.jobs.Submit(ctx, jobs.Spec{
			Kind: k.kind, Title: k.step + " " + name, App: name, ProjectID: projectID, Actor: actor,
			Steps: []string{k.step},
			Run: func(ctx context.Context, t *jobs.Task) error {
				return a.dokku.Lifecycle(ctx, action, name, t.Line)
			},
			Finished: func(ctx context.Context, job store.Job) {
				entry := audit.Entry{
					Action: "apps:" + action, TargetType: "app", TargetName: name, ProjectSlug: projectSlug,
					JobID: job.ID, Result: audit.ResultOK,
				}
				if job.Status != store.JobSucceeded {
					entry.Result = audit.ResultFail
					entry.Detail = map[string]any{"status": job.Status}
				}
				a.audit.Record(ctx, actor, entry)
			},
		})
		if err != nil {
			return err
		}
		writeJSON(w, http.StatusAccepted, JobRef{JobId: job.ID})
		return nil
	}
}

// protectedApps reads the protected_apps setting, a JSON array of app names (SET-2).
func (a *API) protectedApps(ctx context.Context) (map[string]bool, error) {
	raw, ok, err := a.db.Setting(ctx, store.SettingProtectedApps)
	if err != nil || !ok || raw == "" {
		return map[string]bool{}, err
	}
	var names []string
	if err := json.Unmarshal([]byte(raw), &names); err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(names))
	for _, n := range names {
		out[n] = true
	}
	return out, nil
}

var errDokkuUnreachable = newError(http.StatusServiceUnavailable, ErrorCodeDokkuUnreachable,
	"Gokku cannot reach Dokku right now, so actions are disabled.")

// projectRefs maps project ids to references, for job responses.
func (a *API) projectRefs(ctx context.Context) (map[string]ProjectRef, error) {
	projects, err := a.db.Projects(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]ProjectRef, len(projects))
	for _, p := range projects {
		out[p.ID] = ProjectRef{Slug: p.Slug, Name: p.Name}
	}
	return out, nil
}

func toJob(j store.Job, projects map[string]ProjectRef) Job {
	out := Job{
		Id: j.ID, Kind: j.Kind, Title: j.Title, Status: JobStatus(j.Status), App: j.AppName,
		Creator: j.Creator, Source: JobSource(j.Source), Error: j.Error, CreatedAt: j.CreatedAt.Time,
		StartedAt: j.StartedAt.Ptr(), FinishedAt: j.FinishedAt.Ptr(),
	}
	if j.ProjectID != nil {
		if p, ok := projects[*j.ProjectID]; ok {
			out.Project = &p
		}
	}
	return out
}

func toStep(s store.JobStep) JobStep {
	return JobStep{Position: s.Position, Name: s.Name, Status: StepStatus(s.Status),
		StartedAt: s.StartedAt.Ptr(), FinishedAt: s.FinishedAt.Ptr()}
}

const defaultPageSize = 50

// pageLimit reads ?limit= within 1 to 100.
func pageLimit(r *http.Request) (int, error) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return defaultPageSize, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > 100 {
		return 0, validationError(map[string]string{"limit": "Use a number from 1 to 100."})
	}
	return n, nil
}

func (a *API) listJobs(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	limit, err := pageLimit(r)
	if err != nil {
		return err
	}
	q := r.URL.Query()
	f := store.JobFilter{App: q.Get("app"), Cursor: q.Get("cursor"), Limit: limit + 1}
	if slug := q.Get("project"); slug != "" {
		p, err := a.db.ProjectBySlug(ctx, slug)
		if err != nil {
			return projectNotFound(err)
		}
		f.Project = p.ID
	}
	list, err := a.db.ListJobs(ctx, f)
	if err != nil {
		return err
	}
	refs, err := a.projectRefs(ctx)
	if err != nil {
		return err
	}
	page := JobPage{Items: []Job{}}
	if len(list) > limit {
		list = list[:limit]
		next := list[limit-1].ID
		page.NextCursor = &next
	}
	for _, j := range list {
		page.Items = append(page.Items, toJob(j, refs))
	}
	writeJSON(w, http.StatusOK, page)
	return nil
}

func (a *API) jobDetail(ctx context.Context, id string) (JobDetail, error) {
	j, err := a.db.JobByID(ctx, id)
	if err != nil {
		return JobDetail{}, err
	}
	steps, err := a.db.JobSteps(ctx, id)
	if err != nil {
		return JobDetail{}, err
	}
	refs, err := a.projectRefs(ctx)
	if err != nil {
		return JobDetail{}, err
	}
	jj := toJob(j, refs)
	out := JobDetail{
		Id: jj.Id, Kind: jj.Kind, Title: jj.Title, Status: jj.Status, App: jj.App, Project: jj.Project,
		Creator: jj.Creator, Source: JobDetailSource(jj.Source), Error: jj.Error, CreatedAt: jj.CreatedAt,
		StartedAt: jj.StartedAt, FinishedAt: jj.FinishedAt, Steps: make([]JobStep, 0, len(steps)),
	}
	for _, s := range steps {
		out.Steps = append(out.Steps, toStep(s))
	}
	return out, nil
}

func (a *API) getJob(w http.ResponseWriter, r *http.Request) error {
	out, err := a.jobDetail(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		return jobNotFound(err)
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func jobNotFound(err error) error {
	if errors.Is(err, store.ErrNotFound) || errors.Is(err, jobs.ErrNotFound) {
		return newError(http.StatusNotFound, ErrorCodeNotFound, "No job with this id.")
	}
	return err
}

// cancelJob implements JOB-4 and is audited.
func (a *API) cancelJob(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	err := a.jobs.Cancel(ctx, id)
	if errors.Is(err, jobs.ErrFinished) {
		return newError(http.StatusConflict, ErrorCodeJobFinished, "This job has already finished.")
	}
	if err != nil {
		return jobNotFound(err)
	}
	entry := audit.Entry{Action: "jobs:cancel", TargetType: "job", TargetName: id, JobID: id}
	if j, err := a.db.JobByID(ctx, id); err == nil && j.AppName != nil {
		entry.Detail = map[string]any{"app": *j.AppName, "title": j.Title}
	}
	a.audit.Record(ctx, authz.FromContext(ctx), entry)
	w.WriteHeader(http.StatusAccepted)
	return nil
}

// streamJob serves GET /jobs/{id}/stream (JOB-3, EVT-3): the stored output after the
// client's last seq, then live lines, steps and the final status.
func (a *API) streamJob(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	after, _ := strconv.ParseInt(r.Header.Get("Last-Event-ID"), 10, 64)
	if v := r.URL.Query().Get("after"); v != "" {
		after, _ = strconv.ParseInt(v, 10, 64)
	}
	steps, err := a.db.JobSteps(ctx, id)
	if err != nil {
		return err
	}
	backlog, sub, err := a.jobs.Subscribe(ctx, id, after)
	if err != nil {
		return jobNotFound(err)
	}
	if sub != nil {
		defer sub.Close()
	}
	positions := map[int64]int{}
	for _, s := range steps {
		positions[s.ID] = s.Position
	}
	line := func(l store.JobLog) JobLine {
		out := JobLine{Seq: l.Seq, Ts: l.TS.Time, Stream: JobLineStream(l.Stream), Text: l.Line}
		if l.StepID != nil {
			if p, ok := positions[*l.StepID]; ok {
				out.Step = &p
			}
		}
		return out
	}

	s, err := startSSE(w)
	if err != nil {
		return nil //nolint:nilerr // the client went away
	}
	for _, l := range backlog {
		if s.event("line", strconv.FormatInt(l.Seq, 10), line(l)) != nil {
			return nil
		}
	}
	if sub == nil {
		// The job has finished: send its final state and end the stream.
		j, err := a.jobDetail(ctx, id)
		if err == nil {
			_ = s.event("status", "", j)
		}
		return nil
	}

	heartbeat := time.NewTicker(a.heartbeat)
	defer heartbeat.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-heartbeat.C:
			err = s.comment("heartbeat")
		case e, ok := <-sub.C:
			if !ok {
				return nil // dropped or ended; the client resumes with Last-Event-ID
			}
			switch e.Type {
			case "line":
				err = s.event("line", strconv.FormatInt(e.Line.Seq, 10), line(e.Line))
			case "step":
				err = s.event("step", "", toStep(e.Step))
			case "status":
				if j, jerr := a.jobDetail(context.WithoutCancel(ctx), id); jerr == nil {
					_ = s.event("status", "", j)
				}
				return nil
			}
		}
		if err != nil {
			return nil //nolint:nilerr // the client disconnected
		}
	}
}
