package projects

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"

	"gokku/internal/dokku"
	"gokku/internal/reconcile"
	"gokku/internal/secrets"
	"gokku/internal/store"
)

type fixture struct {
	env     *Env
	db      *store.DB
	client  *dokku.Client
	project store.Project
	lines   []string
}

func setup(t *testing.T) *fixture {
	t.Helper()
	db, err := store.OpenMemory(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	box, err := secrets.New(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	client := dokku.NewClient(dokku.NewFakeRunner(time.Now()), slog.New(slog.DiscardHandler))
	p := store.Project{ID: store.NewID(), Slug: "blog", Name: "Blog"}
	if err := db.CreateProject(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	return &fixture{env: NewEnv(db, box, client), db: db, client: client, project: p}
}

func (f *fixture) log(s string) { f.lines = append(f.lines, s) }

// SHV-5: values are stored encrypted; an unchanged value keeps its ciphertext.
func TestSHV5SaveEncrypts(t *testing.T) {
	f := setup(t)
	vars := []Var{{Key: "API_KEY", Value: "very-secret-value", Secret: true}, {Key: "LOG_LEVEL", Value: "info"}}
	if err := f.env.Save(t.Context(), f.project.ID, vars, ""); err != nil {
		t.Fatal(err)
	}
	rows, _ := f.db.ProjectEnv(t.Context(), f.project.ID)
	if bytes.Contains(rows[0].ValueEnc, []byte("very-secret-value")) || !rows[0].IsSecret {
		t.Errorf("stored in clear: %+v", rows[0])
	}
	before := rows[0].ValueEnc
	if err := f.env.Save(t.Context(), f.project.ID, vars, ""); err != nil {
		t.Fatal(err)
	}
	rows, _ = f.db.ProjectEnv(t.Context(), f.project.ID)
	if !bytes.Equal(rows[0].ValueEnc, before) {
		t.Error("an unchanged value was sealed again")
	}
	got, err := f.env.Values(t.Context(), f.project.ID)
	if err != nil || got["API_KEY"] != "very-secret-value" || got["LOG_LEVEL"] != "info" {
		t.Errorf("values = %v, %v", got, err)
	}
}

// SHV-4: reserved keys.
func TestSHV4Reserved(t *testing.T) {
	stores := []string{"postgres", "redis"}
	for key, reserved := range map[string]bool{
		"DOKKU_PROXY_PORT": true, "GOKKU_X": true, "PORT": true, "DATABASE_URL": true, "REDIS_URL": true,
		"MONGO_URL": false, "LOG_LEVEL": false,
	} {
		if (Reserved(key, stores) != "") != reserved {
			t.Errorf("Reserved(%s) = %q", key, Reserved(key, stores))
		}
	}
	for k, ok := range map[string]bool{"LOG_LEVEL": true, "_X": true, "lower": false, "1A": false, "A-B": false} {
		if ValidKey(k) != ok {
			t.Errorf("ValidKey(%q) = %v", k, !ok)
		}
	}
}

// Apply writes into the apps, records provenance, and statuses follow (SHV-6, SHV-7).
func TestApplyStatusesAndRelease(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	vars := map[string]string{"LOG_LEVEL": "info", "TZ": "UTC"}
	apps := []string{"blog-api", "worker-queue"}

	statuses, err := f.env.Statuses(ctx, f.project.ID, vars, apps)
	if err != nil {
		t.Fatal(err)
	}
	// blog-api sets its own TZ in the fake seed.
	if statuses["blog-api"]["LOG_LEVEL"] != reconcile.Pending || statuses["blog-api"]["TZ"] != reconcile.Overridden {
		t.Errorf("before = %v", statuses)
	}
	for _, app := range apps {
		if _, err := f.env.Apply(ctx, f.project.ID, app, vars, f.log); err != nil {
			t.Fatal(err)
		}
	}
	statuses, _ = f.env.Statuses(ctx, f.project.ID, vars, apps)
	if statuses["blog-api"]["LOG_LEVEL"] != reconcile.Applied || statuses["worker-queue"]["TZ"] != reconcile.Applied ||
		statuses["blog-api"]["TZ"] != reconcile.Overridden {
		t.Errorf("after = %v", statuses)
	}
	env, _ := f.client.Config(ctx, "blog-api")
	if env["LOG_LEVEL"] != "info" || env["TZ"] != "Europe/Paris" {
		t.Errorf("blog-api config = %v", env)
	}
	for _, l := range f.lines {
		if strings.Contains(l, "info") || strings.Contains(l, "UTC") {
			t.Errorf("a value reached the job log: %q", l)
		}
	}

	// Applying again changes nothing.
	changed, _ := f.env.Apply(ctx, f.project.ID, "blog-api", vars, f.log)
	if changed {
		t.Error("a second apply wrote again")
	}

	// SHV-2: releasing removes managed keys only.
	changed, err = f.env.Release(ctx, f.project.ID, "worker-queue", nil, f.log)
	if err != nil || !changed {
		t.Fatalf("release = %v, %v", changed, err)
	}
	env, _ = f.client.Config(ctx, "worker-queue")
	if _, ok := env["LOG_LEVEL"]; ok || env["REDIS_URL"] == "" {
		t.Errorf("worker-queue after release = %v", env)
	}
	if p, _ := f.db.AppProvenance(ctx, "worker-queue", f.project.ID); len(p) != 0 {
		t.Errorf("provenance left: %v", p)
	}
}

// PRJ-4: deleting a project releases with the provenance saved before the delete.
func TestPRJ4ReleaseAfterDelete(t *testing.T) {
	f := setup(t)
	ctx := t.Context()
	if _, err := f.env.Apply(ctx, f.project.ID, "storefront", map[string]string{"FEATURE": "on"}, f.log); err != nil {
		t.Fatal(err)
	}
	saved, _ := f.db.ProjectProvenance(ctx, f.project.ID)
	if err := f.db.DeleteProject(ctx, f.project.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.env.Release(ctx, f.project.ID, "storefront", saved["storefront"], f.log); err != nil {
		t.Fatal(err)
	}
	if env, _ := f.client.Config(ctx, "storefront"); env["FEATURE"] != "" {
		t.Errorf("FEATURE survived the project: %v", env)
	}
}
