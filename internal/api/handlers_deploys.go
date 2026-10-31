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

const deployHistoryLimit = 50

func toDeploy(d store.Deploy) Deploy {
	return Deploy{Id: d.ID, Source: DeploySource(d.Source), Ref: d.Ref, Status: DeployStatus(d.Status), JobId: d.JobID,
		Creator: d.Creator, StartedAt: d.StartedAt.Time, FinishedAt: d.FinishedAt.Ptr()}
}

// listAppDeploys serves DEP-3, with the git remote for pushes (DEP-1).
func (a *API) listAppDeploys(w http.ResponseWriter, r *http.Request) error {
	app, ok := a.snap.State().App(chi.URLParam(r, "name"))
	if !ok {
		return newError(http.StatusNotFound, ErrorCodeNotFound, "No app with this name on the server.")
	}
	rows, err := a.db.Deploys(r.Context(), app.Name, deployHistoryLimit)
	if err != nil {
		return err
	}
	out := AppDeploys{GitRemote: a.gitRemote(r, app.Name), Deploys: make([]Deploy, 0, len(rows))}
	if app.DeployBranch != "" {
		out.DeployBranch = &app.DeployBranch
	}
	for _, d := range rows {
		out.Deploys = append(out.Deploys, toDeploy(d))
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// stepTracker moves a deploy job through its steps as Dokku's output reaches them (DEP-2).
type stepTracker struct {
	t       *jobs.Task
	current int
	matched bool
}

func (s *stepTracker) line(l dokku.Line) {
	if step := dokku.DeployStep(l.Text); step > s.current {
		s.t.Step(step)
		s.current, s.matched = step, true
	} else if step == 0 {
		s.matched = true
	}
	s.t.Line(l)
}

// done collapses the steps to one "Deploy" step when no marker was seen.
func (s *stepTracker) done() {
	if !s.matched {
		s.t.Collapse("Deploy")
	}
}

// deployApp implements DEP-1 and DEP-2: an image or repository deploy as a job with steps.
func (a *API) deployApp(w http.ResponseWriter, r *http.Request) error {
	var req DeployRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	state := a.snap.State()
	app, ok := state.App(chi.URLParam(r, "name"))
	if !ok {
		return newError(http.StatusNotFound, ErrorCodeNotFound, "No app with this name on the server.")
	}
	str := func(p *string) string {
		if p == nil {
			return ""
		}
		return strings.TrimSpace(*p)
	}
	image, repo, ref := str(req.Image), str(req.Url), str(req.Ref)
	fields := map[string]string{}
	var source, deployRef, title string
	switch req.Source {
	case DeployRequestSourceImage:
		if !dokku.ValidImage(image) {
			fields["image"] = "Use an image reference such as nginx:1.28 or ghcr.io/acme/app:v2."
		}
		source, deployRef, title = "image", image, "Deploy "+image
	case DeployRequestSourceRepository:
		if !dokku.ValidRepositoryURL(repo) {
			fields["url"] = "Use an https, ssh or git URL of a remote repository, without a password."
		}
		if ref != "" && !dokku.ValidRef(ref) {
			fields["ref"] = "Use a branch, tag or commit."
		}
		source, deployRef, title = "repository", repo, "Deploy "+repo
		if ref != "" {
			deployRef += "#" + ref
			title += " at " + ref
		}
	default:
		fields["source"] = "Choose image or repository."
	}
	if len(fields) > 0 {
		return validationError(fields)
	}
	if !state.Server.Reachable {
		return errDokkuUnreachable
	}
	ctx := r.Context()
	projectID, slug, _ := a.appProject(ctx, app.Name)
	actor := authz.FromContext(ctx)
	name := app.Name
	job, err := a.jobs.Submit(ctx, jobs.Spec{
		Kind: "deploy." + source, Title: title, App: name, ProjectID: projectID, Actor: actor,
		Steps: dokku.DeploySteps,
		Run: func(ctx context.Context, t *jobs.Task) error {
			if err := a.recordDeploy(ctx, t, name, source, deployRef, actor); err != nil {
				return err
			}
			steps := &stepTracker{t: t}
			defer steps.done()
			if source == "image" {
				return a.dokku.DeployImage(ctx, name, image, steps.line)
			}
			return a.dokku.DeployRepository(ctx, name, repo, ref, steps.line)
		},
		Finished: func(ctx context.Context, job store.Job) {
			a.finishDeploy(ctx, job)
			a.auditJob(ctx, actor, job, audit.Entry{Action: "apps:deploy", TargetType: "app", TargetName: name,
				ProjectSlug: slug, Detail: map[string]any{"source": source, "ref": deployRef}})
		},
	})
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusAccepted, JobRef{JobId: job.ID})
	return nil
}

// recordDeploy adds the history row of a deploy job as it starts (DEP-3).
func (a *API) recordDeploy(ctx context.Context, t *jobs.Task, app, source, ref string, actor *authz.Principal) error {
	id := t.JobID()
	d := store.Deploy{ID: store.NewID(), AppName: app, Source: source, Ref: ref, JobID: &id}
	if actor != nil {
		d.CreatedBy = &actor.UserID
	}
	return a.db.CreateDeploy(ctx, d)
}

func (a *API) finishDeploy(ctx context.Context, job store.Job) {
	at := store.Now()
	if job.FinishedAt != nil {
		at = *job.FinishedAt
	}
	if err := a.db.FinishDeploy(ctx, job.ID, job.Status, at); err != nil {
		a.log.Error("finish deploy", "job_id", job.ID, "error", err)
	}
}

// ObserveRevision records a deploy Gokku did not start, such as a git push, from a revision
// change between two snapshots (DEP-3). TODO(spec-1): Dokku keeps no push log; until it does,
// such deploys have no output and their time is when Gokku noticed them.
func ObserveRevision(db *store.DB, log interface{ Error(string, ...any) }) func(ctx context.Context, app, from, to string) {
	return func(ctx context.Context, app, _, to string) {
		if _, err := db.RecordObserved(ctx, app, to, store.Now().Time); err != nil {
			log.Error("record observed deploy", "app", app, "error", err)
		}
	}
}
