package store

import (
	"testing"
)

func TestProjectEnvReplace(t *testing.T) {
	db := newDB(t)
	p := addProject(t, db, "blog")
	set := func(vars ...ProjectEnvVar) {
		t.Helper()
		for i := range vars {
			vars[i].ProjectID = p.ID
		}
		if err := db.ReplaceProjectEnv(t.Context(), p.ID, vars); err != nil {
			t.Fatal(err)
		}
	}
	set(ProjectEnvVar{Key: "LOG_LEVEL", ValueEnc: []byte("a")}, ProjectEnvVar{Key: "TZ", ValueEnc: []byte("b")})
	set(ProjectEnvVar{Key: "TZ", ValueEnc: []byte("c"), IsSecret: true})
	got, err := db.ProjectEnv(t.Context(), p.ID)
	if err != nil || len(got) != 1 || got[0].Key != "TZ" || string(got[0].ValueEnc) != "c" || !got[0].IsSecret {
		t.Fatalf("env = %+v, %v", got, err)
	}
	set()
	if got, _ := db.ProjectEnv(t.Context(), p.ID); len(got) != 0 {
		t.Errorf("env after clearing = %+v", got)
	}
	// The key rule of the schema: upper case, digits and underscores, not starting with a digit.
	if err := db.ReplaceProjectEnv(t.Context(), p.ID, []ProjectEnvVar{{ProjectID: p.ID, Key: "lower", ValueEnc: []byte("x")}}); err == nil {
		t.Error("a lower-case key was stored")
	}
}

func TestProvenanceRenameAndForget(t *testing.T) {
	db := newDB(t)
	p := addProject(t, db, "blog")
	if err := db.AssignApp(t.Context(), "blog-api", p.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateProvenance(t.Context(), "blog-api", p.ID, map[string]string{"A": "h1", "B": "h2"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateProvenance(t.Context(), "blog-api", p.ID, map[string]string{"A": "h3"}, []string{"B"}); err != nil {
		t.Fatal(err)
	}
	got, _ := db.AppProvenance(t.Context(), "blog-api", p.ID)
	if len(got) != 1 || got["A"] != "h3" {
		t.Errorf("provenance = %v", got)
	}

	if err := db.RenameApp(t.Context(), "blog-api", "api.v2"); err != nil {
		t.Fatal(err)
	}
	all, _ := db.ProjectProvenance(t.Context(), p.ID)
	m, _ := db.Memberships(t.Context())
	if all["api.v2"]["A"] != "h3" || m.Apps["api.v2"] != p.ID || m.Apps["blog-api"] != "" {
		t.Errorf("after rename: %v %v", all, m.Apps)
	}

	if err := db.ForgetApp(t.Context(), "api.v2"); err != nil {
		t.Fatal(err)
	}
	all, _ = db.ProjectProvenance(t.Context(), p.ID)
	m, _ = db.Memberships(t.Context())
	if len(all) != 0 || len(m.Apps) != 0 {
		t.Errorf("after forget: %v %v", all, m.Apps)
	}
}
