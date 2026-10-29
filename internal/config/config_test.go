package config

import (
	"log/slog"
	"net/netip"
	"testing"
)

func envMap(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(nil, envMap(nil))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != "127.0.0.1:8080" {
		t.Errorf("Listen = %q", cfg.Listen)
	}
	if cfg.DataDir != "/var/lib/gokku" || cfg.DokkuBin != "/usr/bin/dokku" || !cfg.DokkuSudo {
		t.Errorf("unexpected defaults: %+v", cfg)
	}
	if cfg.Runner != RunnerExec || cfg.LogLevel != slog.LevelInfo || len(cfg.TrustedProxies) != 0 {
		t.Errorf("unexpected defaults: %+v", cfg)
	}
}

func TestLoadPrecedence(t *testing.T) {
	env := envMap(map[string]string{
		"GOKKU_LISTEN":          "0.0.0.0:9000",
		"GOKKU_RUNNER":          "fake",
		"GOKKU_TRUSTED_PROXIES": "127.0.0.1/32, ::1, 10.1.2.3/8",
		"GOKKU_LOG_LEVEL":       "debug",
	})
	cfg, err := Load([]string{"--listen", "127.0.0.1:7000"}, env)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != "127.0.0.1:7000" {
		t.Errorf("flag should win over env, Listen = %q", cfg.Listen)
	}
	if cfg.Runner != RunnerFake || cfg.LogLevel != slog.LevelDebug {
		t.Errorf("env not applied: %+v", cfg)
	}
	want := []netip.Prefix{
		netip.MustParsePrefix("127.0.0.1/32"),
		netip.MustParsePrefix("::1/128"),
		netip.MustParsePrefix("10.0.0.0/8"),
	}
	if len(cfg.TrustedProxies) != len(want) {
		t.Fatalf("TrustedProxies = %v", cfg.TrustedProxies)
	}
	for i := range want {
		if cfg.TrustedProxies[i] != want[i] {
			t.Errorf("TrustedProxies[%d] = %v, want %v", i, cfg.TrustedProxies[i], want[i])
		}
	}
}

func TestLoadInvalid(t *testing.T) {
	tests := map[string]map[string]string{
		"runner":    {"GOKKU_RUNNER": "ssh"},
		"proxies":   {"GOKKU_TRUSTED_PROXIES": "not-an-ip"},
		"log level": {"GOKKU_LOG_LEVEL": "loud"},
		"sudo":      {"GOKKU_DOKKU_SUDO": "maybe"},
	}
	for name, env := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Load(nil, envMap(env)); err == nil {
				t.Error("expected an error")
			}
		})
	}
}

func TestLoadRejectsExtraArgs(t *testing.T) {
	if _, err := Load([]string{"extra"}, envMap(nil)); err == nil {
		t.Error("expected an error")
	}
}
