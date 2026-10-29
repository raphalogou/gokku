package audit

import (
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"gokku/internal/authz"
	"gokku/internal/store"
)

func newRecorder(t *testing.T) (*Recorder, *store.DB) {
	t.Helper()
	db, err := store.OpenMemory(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return New(db, slog.New(slog.NewTextHandler(io.Discard, nil))), db
}

func latest(t *testing.T, db *store.DB) store.AuditEntry {
	t.Helper()
	rows, err := db.AuditEntries(t.Context(), 1)
	if err != nil || len(rows) != 1 {
		t.Fatalf("audit rows: %v, %v", rows, err)
	}
	return rows[0]
}

func TestRecordSessionUser(t *testing.T) { // ACT-1
	r, db := newRecorder(t)
	p := &authz.Principal{UserID: "u1", Username: "alice", Kind: authz.KindSession, Role: authz.RoleAdmin}
	r.Record(t.Context(), p, Entry{Action: "users:invite", TargetType: "user", TargetName: "bob", Detail: map[string]any{"role": "viewer"}})
	e := latest(t, db)
	if e.ActorType != "user" || e.ActorName != "alice" || *e.ActorID != "u1" || e.Source != "web" || e.Result != "ok" {
		t.Errorf("entry = %+v", e)
	}
	if e.Detail != `{"role":"viewer"}` {
		t.Errorf("detail = %s", e.Detail)
	}
}

func TestRecordTokenAndFailure(t *testing.T) {
	r, db := newRecorder(t)
	p := &authz.Principal{UserID: "u1", Username: "alice", Kind: authz.KindToken, TokenName: "ci"}
	r.Record(t.Context(), p, Entry{Action: "users:delete", Result: ResultFail})
	e := latest(t, db)
	if e.ActorType != "token" || e.Source != "api" || e.Result != "fail" {
		t.Errorf("entry = %+v", e)
	}
	var d map[string]string
	if err := json.Unmarshal([]byte(e.Detail), &d); err != nil || d["via_token"] != "ci" {
		t.Errorf("detail = %s", e.Detail)
	}
}

func TestRecordAnonymousAndSystem(t *testing.T) {
	r, db := newRecorder(t)
	r.Record(t.Context(), nil, Entry{Action: "auth:login", ActorName: "mallory", Result: ResultFail})
	if e := latest(t, db); e.ActorType != "user" || e.ActorName != "mallory" || e.ActorID != nil {
		t.Errorf("anonymous entry = %+v", e)
	}
	r.RecordSystem(t.Context(), Entry{Action: "users:reset-password", TargetName: "alice"})
	if e := latest(t, db); e.ActorType != "system" || e.Source != "system" {
		t.Errorf("system entry = %+v", e)
	}
}

func TestRedactSecrets(t *testing.T) { // SEC-5
	got := Redact(map[string]any{
		"username": "bob",
		"password": "hunter2",
		"nested":   map[string]any{"DATABASE_URL": "postgres://x", "role": "viewer"},
		"apiToken": "gk_abc",
	})
	b, _ := json.Marshal(got)
	want := `{"apiToken":"[redacted]","nested":{"DATABASE_URL":"[redacted]","role":"viewer"},"password":"[redacted]","username":"bob"}`
	if string(b) != want {
		t.Errorf("Redact = %s", b)
	}
}
