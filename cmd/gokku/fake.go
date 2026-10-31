package main

import (
	"context"
	"fmt"
	"time"

	"gokku/internal/store"
)

// fakeDelay paces the fake runner's streamed output, so job progress is visible.
const fakeDelay = 150 * time.Millisecond

// fakeProjects are the projects of the mockup, seeded once for GOKKU_RUNNER=fake (NFR-8).
var fakeProjects = []struct {
	slug, name, description string
	apps                    []string
	services                []store.ServiceKey
}{
	{"blog-platform", "Blog platform", "Public blog, API and background jobs",
		[]string{"blog-api", "worker-queue"}, []store.ServiceKey{{Type: "postgres", Name: "blog-db"}, {Type: "redis", Name: "blog-cache"}}},
	{"storefront", "Storefront", "Shop frontend and staging copy",
		[]string{"storefront", "staging-web"}, []store.ServiceKey{{Type: "postgres", Name: "shop-db"}}},
	{"internal-tools", "Internal tools", "Docs and status pages",
		[]string{"docs-site", "status-page"}, nil},
}

// seedFake fills a new database with the mockup's projects and a failed deploy of
// status-page, so every app status shows up. It runs once per database.
func seedFake(ctx context.Context, db *store.DB) error {
	if _, done, err := db.Setting(ctx, store.SettingFakeSeeded); err != nil || done {
		return err
	}
	for _, fp := range fakeProjects {
		p := store.Project{ID: store.NewID(), Slug: fp.slug, Name: fp.name, Description: fp.description}
		if err := db.CreateProject(ctx, p); err != nil {
			return fmt.Errorf("seed project %s: %w", fp.slug, err)
		}
		for _, app := range fp.apps {
			if err := db.AssignApp(ctx, app, p.ID); err != nil {
				return err
			}
		}
		for _, s := range fp.services {
			if err := db.AssignService(ctx, s, p.ID); err != nil {
				return err
			}
		}
	}
	failed := store.At(time.Now().Add(-20 * time.Minute))
	if _, err := db.W.ExecContext(ctx, `
		INSERT INTO jobs (id, kind, title, status, app_name, source, error, created_at, started_at, finished_at)
		VALUES (?, 'deploy.git', 'Deploy status-page', 'failed', 'status-page', 'system', 'build failed', ?, ?, ?)`,
		store.NewID(), failed, failed, failed); err != nil {
		return fmt.Errorf("seed job: %w", err)
	}
	return db.SetSetting(ctx, store.SettingFakeSeeded, "1")
}
