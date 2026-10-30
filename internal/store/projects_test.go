package store

import (
	"errors"
	"testing"
)

func addProject(t *testing.T, db *DB, slug string) Project {
	t.Helper()
	p := Project{ID: NewID(), Slug: slug, Name: slug}
	if err := db.CreateProject(t.Context(), p); err != nil {
		t.Fatalf("create %s: %v", slug, err)
	}
	return p
}

func TestProjectSlugIsUniqueAndChecked(t *testing.T) {
	db := newDB(t)
	addProject(t, db, "blog")
	err := db.CreateProject(t.Context(), Project{ID: NewID(), Slug: "blog", Name: "x"})
	if !errors.Is(err, ErrConflict) {
		t.Errorf("duplicate slug: %v, want ErrConflict", err)
	}
	for _, slug := range []string{"", "Blog", "-blog", "blog_platform", "blog platform"} {
		if err := db.CreateProject(t.Context(), Project{ID: NewID(), Slug: slug, Name: "x"}); err == nil {
			t.Errorf("slug %q was accepted", slug)
		}
	}
}

func TestProjectUpdateAndDelete(t *testing.T) {
	db := newDB(t)
	p := addProject(t, db, "blog")
	if err := db.UpdateProject(t.Context(), p.ID, "Blog platform", "Public blog"); err != nil {
		t.Fatal(err)
	}
	got, err := db.ProjectBySlug(t.Context(), "blog")
	if err != nil || got.Name != "Blog platform" || got.Description != "Public blog" {
		t.Fatalf("after update: %+v, %v", got, err)
	}
	if err := db.UpdateProject(t.Context(), "missing", "x", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("update missing: %v", err)
	}

	if err := db.AssignApp(t.Context(), "blog-api", p.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.AssignService(t.Context(), ServiceKey{"postgres", "blog-db"}, p.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteProject(t.Context(), p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ProjectBySlug(t.Context(), "blog"); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted project still found: %v", err)
	}
	m, err := db.Memberships(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Apps) != 0 || len(m.Services) != 0 {
		t.Errorf("membership rows survived the project: %+v", m)
	}
}

// PRJ-2: an app and a service each belong to at most one project; assigning moves them.
func TestPRJ2MembershipMovesBetweenProjects(t *testing.T) {
	db := newDB(t)
	a, b := addProject(t, db, "a"), addProject(t, db, "b")
	svc := ServiceKey{"redis", "cache"}
	for _, step := range []string{a.ID, a.ID, b.ID} {
		if err := db.AssignApp(t.Context(), "web", step); err != nil {
			t.Fatal(err)
		}
		if err := db.AssignService(t.Context(), svc, step); err != nil {
			t.Fatal(err)
		}
	}
	m, err := db.Memberships(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if m.Apps["web"] != b.ID || m.Services[svc] != b.ID || len(m.Apps) != 1 || len(m.Services) != 1 {
		t.Fatalf("membership = %+v, want web and cache in b only", m)
	}

	if err := db.ReleaseApp(t.Context(), "web", a.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("release from the wrong project: %v, want ErrNotFound", err)
	}
	if err := db.ReleaseApp(t.Context(), "web", b.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.ReleaseService(t.Context(), svc, b.ID); err != nil {
		t.Fatal(err)
	}
	m, _ = db.Memberships(t.Context())
	if len(m.Apps) != 0 || len(m.Services) != 0 {
		t.Errorf("after release: %+v", m)
	}
	// Names follow Dokku's rules (migration 00002).
	for app, ok := range map[string]bool{"api.v2": true, "blog_api": false, "Blog": false, "-x": false} {
		if err := db.AssignApp(t.Context(), app, a.ID); (err == nil) != ok {
			t.Errorf("AssignApp(%q) = %v", app, err)
		}
	}
	for name, ok := range map[string]bool{"Blog_DB": true, "-db": false, "db.x": false} {
		if err := db.AssignService(t.Context(), ServiceKey{"postgres", name}, a.ID); (err == nil) != ok {
			t.Errorf("AssignService(%q) = %v", name, err)
		}
	}
}

func TestProjectAuditEntries(t *testing.T) {
	db := newDB(t)
	for _, slug := range []string{"blog", "shop", "blog"} {
		if err := db.InsertAudit(t.Context(), AuditEntry{TS: Now(), ActorType: "system", ActorName: "system",
			Action: "x", ProjectSlug: slug, Result: "ok", Source: "system", Detail: "{}"}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := db.ProjectAuditEntries(t.Context(), "blog", 10)
	if err != nil || len(got) != 2 || got[0].ID < got[1].ID {
		t.Errorf("entries = %+v, %v", got, err)
	}
}
