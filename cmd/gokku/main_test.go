package main

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gokku/internal/auth"
	"gokku/internal/config"
)

func TestRunVersion(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"version"}, nil, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != version {
		t.Errorf("output = %q, want %q", out.String(), version)
	}
}

func TestRunRejectsUnknownAndMissingCommands(t *testing.T) {
	for _, args := range [][]string{nil, {"deploy"}, {"admin"}, {"admin", "nuke"}} {
		var stderr bytes.Buffer
		if err := run(args, nil, &bytes.Buffer{}, &stderr); err == nil {
			t.Errorf("run(%q) succeeded", args)
		}
		if !strings.Contains(stderr.String(), "Usage: gokku") {
			t.Errorf("run(%q) did not print usage", args)
		}
	}
}

func TestResetPasswordCommand(t *testing.T) { // AUTH-8
	dir := t.TempDir()
	t.Setenv("GOKKU_DATA_DIR", dir)
	t.Setenv("GOKKU_LOG_LEVEL", "error")

	// Create an admin through the service, as the setup screen would.
	ctx := t.Context()
	db, _, err := openState(ctx, config.Config{DataDir: dir}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := auth.New(db, auth.Options{Argon2: auth.Argon2Params{Memory: 64, Time: 1, Threads: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Setup(ctx, "alice", "first password!", auth.ClientInfo{}); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	var out bytes.Buffer
	if err := run([]string{"admin", "reset-password", "alice"}, strings.NewReader("recovered password\n"), &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Password for alice changed") {
		t.Errorf("output = %q", out.String())
	}

	err = run([]string{"admin", "reset-password", "alice"}, strings.NewReader("short\n"), &out, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "at least 12") {
		t.Errorf("short password: %v", err)
	}
	err = run([]string{"admin", "reset-password", "nobody"}, strings.NewReader("recovered password\n"), &out, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), `no user named "nobody"`) {
		t.Errorf("unknown user: %v", err)
	}

	db, _, err = openState(ctx, config.Config{DataDir: dir}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	svc, _ = auth.New(db, auth.Options{Argon2: auth.Argon2Params{Memory: 64, Time: 1, Threads: 1}})
	if _, _, err := svc.Login(ctx, "alice", "recovered password", auth.ClientInfo{}); err != nil {
		t.Errorf("sign in with the reset password: %v", err)
	}
}

func TestOpenStateDetectsReplacedKey(t *testing.T) { // SEC-4
	dir := t.TempDir()
	cfg := config.Config{DataDir: dir}
	log := slog.New(slog.DiscardHandler)
	db, _, err := openState(t.Context(), cfg, log)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	if err := os.WriteFile(filepath.Join(dir, "secret.key"), bytes.Repeat([]byte{1}, 32), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := openState(t.Context(), cfg, log); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Errorf("replaced key: %v", err)
	}
}
