package dokku

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestParseConfigFixture(t *testing.T) {
	env, err := parseConfigJSON(fixture(t, "config-export.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(env) != 5 || env["LOG_LEVEL"] != "info" || env["MULTI_LINE"] != "line one\nline two" || env["QUOTED"] != `it's "quoted"` {
		t.Errorf("env = %v", env)
	}
	if env, err := parseConfigJSON(""); err != nil || len(env) != 0 {
		t.Errorf("empty output = %v, %v", env, err)
	}
	if _, err := parseConfigJSON("export A='1'"); err == nil {
		t.Error("non-JSON output must be an error")
	}
}

func TestValidConfigKey(t *testing.T) {
	for k, want := range map[string]bool{"LOG_LEVEL": true, "_x": true, "lower": true, "A1": true,
		"1A": false, "A-B": false, "A B": false, "": false, "A=B": false} {
		if ValidConfigKey(k) != want {
			t.Errorf("ValidConfigKey(%q) = %v", k, !want)
		}
	}
}

// CFG-4: values go on stdin, not in process arguments, and never reach errors or logs.
func TestCFG4SetConfigUsesStdin(t *testing.T) {
	fake := NewFakeRunner(time.Now())
	cl := NewClient(fake, discard())
	secret := "s3cr3t-value-123"
	if err := cl.SetConfig(t.Context(), "blog-api", map[string]string{"API_TOKEN": secret, "LOG_LEVEL": "debug"}); err != nil {
		t.Fatal(err)
	}
	for _, call := range fake.Calls() {
		if strings.Contains(call, secret) {
			t.Errorf("a secret reached the command line: %q", call)
		}
	}
	if !slices.Contains(fake.Calls(), "config:import --no-restart --format json blog-api -") {
		t.Errorf("calls = %v", fake.Calls())
	}
	env, err := cl.Config(t.Context(), "blog-api")
	if err != nil || env["API_TOKEN"] != secret || env["LOG_LEVEL"] != "debug" || env["DATABASE_URL"] == "" {
		t.Errorf("env = %v, %v", env, err)
	}

	if err := cl.UnsetConfig(t.Context(), "blog-api", []string{"API_TOKEN", "LOG_LEVEL"}); err != nil {
		t.Fatal(err)
	}
	env, _ = cl.Config(t.Context(), "blog-api")
	if _, ok := env["API_TOKEN"]; ok {
		t.Error("API_TOKEN survived the unset")
	}

	for _, bad := range []map[string]string{{"A-B": "x"}, {"1X": "y"}} {
		if err := cl.SetConfig(t.Context(), "blog-api", bad); !errors.Is(err, ErrInvalidName) {
			t.Errorf("%v accepted: %v", bad, err)
		}
	}
	if err := cl.UnsetConfig(t.Context(), "blog-api", []string{"--global"}); !errors.Is(err, ErrInvalidName) {
		t.Errorf("a flag was accepted as a key: %v", err)
	}
}

func TestSecretValuesAreScrubbed(t *testing.T) {
	c := newCmd("config:import", "app", "-").secretValue("hunter2-long").secretValue("1")
	if got := c.scrub("Setting A=hunter2-long and B=1"); got != "Setting A="+redacted+" and B=1" {
		t.Errorf("scrub = %q (short values are left alone)", got)
	}
}

func TestServiceDSNOnFake(t *testing.T) {
	cl := NewClient(NewFakeRunner(time.Now()), discard())
	dsn, err := cl.ServiceDSN(t.Context(), "postgres", "blog-db")
	if err != nil || !strings.HasPrefix(dsn, "postgres://") {
		t.Errorf("dsn = %q, %v", dsn, err)
	}
	env, _ := cl.Config(t.Context(), "blog-api")
	if env["DATABASE_URL"] != dsn {
		t.Error("the fake link did not set DATABASE_URL to the DSN")
	}
	if _, err := cl.ServiceDSN(t.Context(), "sqlite", "x"); !errors.Is(err, ErrInvalidName) {
		t.Errorf("sqlite: %v", err)
	}
}
