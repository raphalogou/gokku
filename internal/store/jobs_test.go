package store

import (
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
