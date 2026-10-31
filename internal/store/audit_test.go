package store

import (
	"testing"
)

func addAudit(t *testing.T, db *DB, actor, action, target, project, result, source string) {
	t.Helper()
	if err := db.InsertAudit(t.Context(), AuditEntry{TS: Now(), ActorType: "user", ActorName: actor, Action: action,
		TargetName: target, ProjectSlug: project, Result: result, Source: source, Detail: "{}"}); err != nil {
		t.Fatal(err)
	}
}

// ACT-2: filters, text search and cursor pages.
func TestACT2FindAudit(t *testing.T) {
	db := newDB(t)
	addAudit(t, db, "maya", "apps:restart", "blog-api", "blog", "ok", "web")
	addAudit(t, db, "alex", "config:set", "storefront", "shop", "ok", "web")
	addAudit(t, db, "ci", "apps:rebuild", "storefront", "shop", "fail", "api")
	addAudit(t, db, "maya", "users:invite", "jo_rdan", "", "ok", "web")

	count := func(f AuditFilter) int {
		t.Helper()
		f.Limit = 50
		got, err := db.FindAudit(t.Context(), f)
		if err != nil {
			t.Fatal(err)
		}
		return len(got)
	}
	for name, c := range map[string]struct {
		f    AuditFilter
		want int
	}{
		"all":      {AuditFilter{}, 4},
		"actor":    {AuditFilter{Actor: "maya"}, 2},
		"project":  {AuditFilter{Project: "shop"}, 2},
		"result":   {AuditFilter{Result: "fail"}, 1},
		"source":   {AuditFilter{Source: "api"}, 1},
		"query":    {AuditFilter{Query: "store"}, 2},
		"combined": {AuditFilter{Query: "apps:", Project: "shop"}, 1},
		// LIKE wildcards in the search are literal.
		"wildcard": {AuditFilter{Query: "o_r"}, 1},
		"percent":  {AuditFilter{Query: "%"}, 0},
	} {
		if got := count(c.f); got != c.want {
			t.Errorf("%s: %d, want %d", name, got, c.want)
		}
	}

	page, _ := db.FindAudit(t.Context(), AuditFilter{Limit: 3})
	next, _ := db.FindAudit(t.Context(), AuditFilter{Limit: 3, Cursor: page[2].ID})
	if len(page) != 3 || len(next) != 1 || next[0].Action != "apps:restart" {
		t.Errorf("pages = %d + %d", len(page), len(next))
	}

	var all []string
	if err := db.EachAudit(t.Context(), AuditFilter{Actor: "maya"}, func(e AuditEntry) error {
		all = append(all, e.Action)
		return nil
	}); err != nil || len(all) != 2 || all[0] != "users:invite" {
		t.Errorf("each = %v, %v", all, err)
	}
	actors, _ := db.AuditActors(t.Context())
	if len(actors) != 3 || actors[0] != "alex" {
		t.Errorf("actors = %v", actors)
	}
}
