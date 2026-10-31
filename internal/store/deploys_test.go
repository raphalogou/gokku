package store

import (
	"testing"
	"time"
)

// DEP-3: Gokku's own deploys and observed ones, newest first.
func TestDEP3Deploys(t *testing.T) {
	db := newDB(t)
	ctx := t.Context()
	j := newJob(t, db, "blog-api")
	if err := db.CreateDeploy(ctx, Deploy{ID: NewID(), AppName: "blog-api", Source: "image", Ref: "nginx:1", JobID: &j.ID}); err != nil {
		t.Fatal(err)
	}
	// A revision change during or just after Gokku's deploy is that deploy.
	if ok, err := db.RecordObserved(ctx, "blog-api", "abc", time.Now()); err != nil || ok {
		t.Errorf("observed during a deploy: %v, %v", ok, err)
	}
	if err := db.FinishDeploy(ctx, j.ID, JobSucceeded, Now()); err != nil {
		t.Fatal(err)
	}
	if ok, _ := db.RecordObserved(ctx, "blog-api", "abc", time.Now()); ok {
		t.Error("observed right after a deploy")
	}
	if ok, err := db.RecordObserved(ctx, "blog-api", "def", time.Now().Add(observedGrace+time.Minute)); err != nil || !ok {
		t.Errorf("observed later: %v, %v", ok, err)
	}
	if ok, _ := db.RecordObserved(ctx, "docs-site", "123", time.Now()); !ok {
		t.Error("an app without Gokku deploys was not observed")
	}
	got, err := db.Deploys(ctx, "blog-api", 10)
	if err != nil || len(got) != 2 || got[0].Source != "observed" || got[0].Ref != "def" || got[1].Status != JobSucceeded {
		t.Errorf("deploys = %+v, %v", got, err)
	}
}
