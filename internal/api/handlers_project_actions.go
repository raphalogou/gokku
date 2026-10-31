package api

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"github.com/go-chi/chi/v5"

	"gokku/internal/audit"
	"gokku/internal/authz"
	"gokku/internal/dokku"
	"gokku/internal/jobs"
	"gokku/internal/store"
)

const projectActionKind = "project."

// projectAction returns the handler of POST /projects/{slug}/actions/<action> (PRJ-5).
func (a *API) projectAction(action string) HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		ctx := r.Context()
		p, err := a.db.ProjectBySlug(ctx, chi.URLParam(r, "slug"))
		if err != nil {
			return projectNotFound(err)
		}
		apps, err := a.memberApps(ctx, p.ID)
		if err != nil {
			return err
		}
		if len(apps) == 0 {
			return validationError(map[string]string{"project": "The project has no apps."})
		}
		job, err := a.submitProjectAction(ctx, p, action, apps)
		if err != nil {
			return err
		}
		writeJSON(w, http.StatusAccepted, JobRef{JobId: job.ID})
		return nil
	}
}

// submitProjectAction queues one parent job with a child per app (JOB-5). Protected apps
// are left out of stop and rebuild (SET-2), and the parent's output says so.
func (a *API) submitProjectAction(ctx context.Context, p store.Project, action string, apps []string) (store.Job, error) {
	if !a.snap.State().Server.Reachable {
		return store.Job{}, errDokkuUnreachable
	}
	var skipped []string
	if slices.Contains(protectedActions, action) {
		protected, err := a.protectedApps(ctx)
		if err != nil {
			return store.Job{}, err
		}
		apps = slices.DeleteFunc(slices.Clone(apps), func(app string) bool {
			if protected[app] {
				skipped = append(skipped, app)
				return true
			}
			return false
		})
		if len(apps) == 0 {
			return store.Job{}, errAppProtected
		}
	}
	children := make([]jobs.Spec, 0, len(apps))
	for _, app := range apps {
		spec, err := a.lifecycleSpec(ctx, action, app)
		if err != nil {
			return store.Job{}, err
		}
		children = append(children, spec)
	}
	actor := authz.FromContext(ctx)
	verb := lifecycleKinds[action].step
	detail := map[string]any{"apps": apps}
	if len(skipped) > 0 {
		detail["skipped_protected"] = skipped
	}
	return a.jobs.SubmitGroup(ctx, jobs.Spec{
		Kind: projectActionKind + action, Title: verb + " all apps in " + p.Name, ProjectID: p.ID, Actor: actor,
		Finished: func(ctx context.Context, job store.Job) {
			a.auditJob(ctx, actor, job, audit.Entry{Action: "projects:" + action, TargetType: "project",
				TargetName: p.Slug, ProjectSlug: p.Slug, Detail: detail})
		},
	}, children)
}

// retryFailedJobs runs a finished project action again for the apps whose job did not
// succeed, as a new parent (JOB-5).
func (a *API) retryFailedJobs(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	parent, err := a.db.JobByID(ctx, chi.URLParam(r, "id"))
	if err != nil {
		return jobNotFound(err)
	}
	action, ok := strings.CutPrefix(parent.Kind, projectActionKind)
	if !ok || !dokku.IsLifecycleAction(action) || parent.ProjectID == nil {
		return newError(http.StatusConflict, ErrorCodeNothingToRetry, "Only project actions can be retried.")
	}
	if parent.Status == store.JobQueued || parent.Status == store.JobRunning {
		return newError(http.StatusConflict, ErrorCodeNothingToRetry, "The job is still running.")
	}
	p, err := a.db.ProjectByID(ctx, *parent.ProjectID)
	if err != nil {
		return projectNotFound(err)
	}
	children, err := a.db.ChildJobs(ctx, parent.ID)
	if err != nil {
		return err
	}
	state := a.snap.State()
	var apps []string
	for _, c := range children {
		if c.Status != store.JobSucceeded && c.AppName != nil {
			if _, exists := state.App(*c.AppName); exists {
				apps = append(apps, *c.AppName)
			}
		}
	}
	if len(apps) == 0 {
		return newError(http.StatusConflict, ErrorCodeNothingToRetry, "No app failed in this job.")
	}
	job, err := a.submitProjectAction(ctx, p, action, apps)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusAccepted, JobRef{JobId: job.ID})
	return nil
}
