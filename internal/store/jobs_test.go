package store

import (
	"errors"
	"testing"
	"time"
)

func addJob(t *testing.T, db *DB, app, kind, status string, finished time.Time) {
	t.Helper()
	var fin *Time
	if !finished.IsZero() {
		f := At(finished)
		fin = &f
	}
	_, err := db.W.ExecContext(t.Context(),
		`INSERT INTO jobs (id, kind, title, status, app_name, finished_at) VALUES (?, ?, ?, ?, ?, ?)`,
		NewID(), kind, kind, status, app, fin)
	if err != nil {
		t.Fatal(err)
	}
}

func TestAppJobStates(t *testing.T) {
	db := newDB(t)
	now := time.Now()
	addJob(t, db, "a", "deploy.git", "failed", now.Add(-time.Hour))
	addJob(t, db, "a", "app.restart", "succeeded", now) // a newer success clears the failure
	addJob(t, db, "b", "app.restart", "succeeded", now.Add(-time.Hour))
	addJob(t, db, "b", "deploy.image", "failed", now)
	addJob(t, db, "c", "app.rebuild", "running", time.Time{})
	addJob(t, db, "d", "app.config", "failed", now) // other kinds do not count

	states, err := db.AppJobStates(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]AppJobState{}
	for _, s := range states {
		got[s.App] = s
	}
	want := map[string]AppJobState{
		"a": {App: "a"},
		"b": {App: "b", LastFailed: true},
		"c": {App: "c", Running: true},
	}
	if len(got) != len(want) {
		t.Fatalf("states = %+v", got)
	}
	for app, w := range want {
		if got[app] != w {
			t.Errorf("%s = %+v, want %+v", app, got[app], w)
		}
	}
}

func newJob(t *testing.T, db *DB, app string, steps ...string) Job {
	t.Helper()
	j := Job{ID: NewID(), Kind: JobKindRestart, Title: "Restart " + app, Status: JobQueued, AppName: &app, Source: "web"}
	if err := db.CreateJob(t.Context(), j, steps); err != nil {
		t.Fatal(err)
	}
	return j
}

// JOB-1, JOB-3: a job moves through its states; output is stored by sequence number.
func TestJobLifecycleAndLogs(t *testing.T) {
	db := newDB(t)
	j := newJob(t, db, "blog-api", "Restart", "Check")
	steps, err := db.JobSteps(t.Context(), j.ID)
	if err != nil || len(steps) != 2 || steps[0].Status != JobQueued || steps[1].Name != "Check" {
		t.Fatalf("steps = %+v, %v", steps, err)
	}
	if err := db.StartJob(t.Context(), j.ID, Now()); err != nil {
		t.Fatal(err)
	}
	if err := db.StartJob(t.Context(), j.ID, Now()); err == nil {
		t.Error("a running job was started again")
	}
	if err := db.SetStepStatus(t.Context(), steps[0].ID, JobRunning, Now()); err != nil {
		t.Fatal(err)
	}
	step := steps[0].ID
	if err := db.AppendJobLogs(t.Context(), []JobLog{
		{JobID: j.ID, Seq: 1, StepID: &step, TS: Now(), Stream: "stdout", Line: "-----> Restarting"},
		{JobID: j.ID, Seq: 2, StepID: &step, TS: Now(), Stream: "stderr", Line: " !     oops"},
	}); err != nil {
		t.Fatal(err)
	}
	msg := "exit status 1"
	if err := db.FinishJob(t.Context(), j.ID, JobFailed, &msg, Now()); err != nil {
		t.Fatal(err)
	}

	got, err := db.JobByID(t.Context(), j.ID)
	if err != nil || got.Status != JobFailed || got.Error == nil || *got.Error != msg || got.FinishedAt == nil || got.StartedAt == nil {
		t.Errorf("job = %+v, %v", got, err)
	}
	steps, _ = db.JobSteps(t.Context(), j.ID)
	if steps[0].Status != JobFailed || steps[1].Status != StepSkipped {
		t.Errorf("steps after failure = %+v", steps)
	}
	logs, err := db.JobLogsAfter(t.Context(), j.ID, 1)
	if err != nil || len(logs) != 1 || logs[0].Seq != 2 || logs[0].Stream != "stderr" {
		t.Errorf("logs after 1 = %+v, %v", logs, err)
	}
	if _, err := db.JobByID(t.Context(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing job: %v", err)
	}
}

func TestListJobsPages(t *testing.T) {
	db := newDB(t)
	var ids []string
	for i := range 5 {
		app := "a"
		if i%2 == 1 {
			app = "b"
		}
		ids = append(ids, newJob(t, db, app).ID)
	}
	page, err := db.ListJobs(t.Context(), JobFilter{Limit: 2})
	if err != nil || len(page) != 2 || page[0].ID != ids[4] || page[1].ID != ids[3] {
		t.Fatalf("page 1 = %v, %v", page, err)
	}
	page, _ = db.ListJobs(t.Context(), JobFilter{Limit: 2, Cursor: page[1].ID})
	if len(page) != 2 || page[0].ID != ids[2] {
		t.Errorf("page 2 = %v", page)
	}
	page, _ = db.ListJobs(t.Context(), JobFilter{Limit: 10, App: "b"})
	if len(page) != 2 {
		t.Errorf("app b = %d jobs", len(page))
	}
}

// JOB-6: jobs left queued or running by a previous process are failed as interrupted.
func TestJOB6MarkInterrupted(t *testing.T) {
	db := newDB(t)
	queued := newJob(t, db, "a", "Restart")
	running := newJob(t, db, "b", "Restart")
	done := newJob(t, db, "c")
	_ = db.StartJob(t.Context(), running.ID, Now())
	_ = db.StartJob(t.Context(), done.ID, Now())
	_ = db.FinishJob(t.Context(), done.ID, JobSucceeded, nil, Now())

	n, err := db.MarkInterrupted(t.Context(), Now())
	if err != nil || n != 2 {
		t.Fatalf("interrupted %d, %v", n, err)
	}
	for _, id := range []string{queued.ID, running.ID} {
		j, _ := db.JobByID(t.Context(), id)
		if j.Status != JobFailed || j.Error == nil || *j.Error != "interrupted" {
			t.Errorf("%s = %+v", id, j)
		}
		steps, _ := db.JobSteps(t.Context(), id)
		if steps[0].Status != JobFailed {
			t.Errorf("step of %s = %s", id, steps[0].Status)
		}
	}
	if j, _ := db.JobByID(t.Context(), done.ID); j.Status != JobSucceeded {
		t.Errorf("a finished job changed: %+v", j)
	}
}

// JOB-7: old jobs are deleted, but the latest 20 of each app stay.
func TestJOB7PruneJobs(t *testing.T) {
	db := newDB(t)
	old := time.Now().Add(-40 * 24 * time.Hour)
	for i := range 25 {
		app := "busy"
		if i >= 22 {
			app = "quiet"
		}
		id := NewID()
		if _, err := db.W.ExecContext(t.Context(), `INSERT INTO jobs (id, kind, title, status, app_name, created_at)
			VALUES (?, 'app.restart', 'x', 'succeeded', ?, ?)`, id, app, At(old.Add(time.Duration(i)*time.Minute))); err != nil {
			t.Fatal(err)
		}
		_ = db.AppendJobLogs(t.Context(), []JobLog{{JobID: id, Seq: 1, TS: Now(), Stream: "stdout", Line: "x"}})
	}
	recent := newJob(t, db, "busy")
	n, err := db.PruneJobs(t.Context(), time.Now().Add(-30*24*time.Hour), 20)
	if err != nil {
		t.Fatal(err)
	}
	// busy has 22 old jobs and 1 recent: keeps the recent one and the 19 newest old ones.
	if n != 3 {
		t.Errorf("pruned %d, want 3", n)
	}
	busy, _ := db.ListJobs(t.Context(), JobFilter{App: "busy", Limit: 100})
	quiet, _ := db.ListJobs(t.Context(), JobFilter{App: "quiet", Limit: 100})
	if len(busy) != 20 || busy[0].ID != recent.ID || len(quiet) != 3 {
		t.Errorf("busy %d, quiet %d", len(busy), len(quiet))
	}
	var orphans int
	_ = db.R.Get(&orphans, `SELECT COUNT(*) FROM job_logs WHERE job_id NOT IN (SELECT id FROM jobs)`)
	if orphans != 0 {
		t.Errorf("%d log lines outlived their job", orphans)
	}
}
