// Package config loads Gokku's configuration from environment variables and flags.
// Flags win over environment variables, which win over defaults (DESIGN.md, section 3.8).
package config

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"strconv"
	"strings"
)

// Config is the resolved configuration of a Gokku process.
type Config struct {
	Listen         string
	DataDir        string
	DokkuBin       string
	DokkuSudo      bool
	DokkuHost      string
	Runner         string
	TrustedProxies []netip.Prefix
	LogLevel       slog.Level
}

// Runner implementations selectable with GOKKU_RUNNER.
const (
	RunnerExec = "exec"
	RunnerFake = "fake"
)

// Load parses args (without the program and subcommand names) on top of the environment.
// lookupEnv is os.LookupEnv in production and a map lookup in tests. Positional arguments
// are an error.
func Load(args []string, lookupEnv func(string) (string, bool)) (Config, error) {
	cfg, rest, err := LoadArgs(args, lookupEnv)
	if err == nil && len(rest) > 0 {
		err = fmt.Errorf("unexpected arguments: %s", strings.Join(rest, " "))
	}
	return cfg, err
}

// LoadArgs is Load for commands that take positional arguments after the flags, which it
// returns.
func LoadArgs(args []string, lookupEnv func(string) (string, bool)) (Config, []string, error) {
	env := func(key, def string) string {
		if v, ok := lookupEnv(key); ok && v != "" {
			return v
		}
		return def
	}

	fs := flag.NewFlagSet("gokku", flag.ContinueOnError)
	listen := fs.String("listen", env("GOKKU_LISTEN", "127.0.0.1:8080"), "listen address")
	dataDir := fs.String("data-dir", env("GOKKU_DATA_DIR", "/var/lib/gokku"), "database and secret key directory")
	dokkuBin := fs.String("dokku-bin", env("GOKKU_DOKKU_BIN", "/usr/bin/dokku"), "path of the dokku binary")
	dokkuSudo := fs.String("dokku-sudo", env("GOKKU_DOKKU_SUDO", "true"), "run dokku through sudo -n")
	dokkuHost := fs.String("dokku-host", env("GOKKU_DOKKU_HOST", ""), "host shown in git remote instructions (default: request host)")
	runner := fs.String("runner", env("GOKKU_RUNNER", RunnerExec), "dokku runner: exec or fake")
	proxies := fs.String("trusted-proxies", env("GOKKU_TRUSTED_PROXIES", ""), "comma-separated CIDRs allowed to set X-Forwarded-*")
	logLevel := fs.String("log-level", env("GOKKU_LOG_LEVEL", "info"), "log level: debug, info, warn, error")
	if err := fs.Parse(args); err != nil {
		return Config{}, nil, err
	}

	cfg := Config{
		Listen:    *listen,
		DataDir:   *dataDir,
		DokkuBin:  *dokkuBin,
		DokkuHost: *dokkuHost,
		Runner:    *runner,
	}

	var errs []error
	sudo, err := strconv.ParseBool(*dokkuSudo)
	if err != nil {
		errs = append(errs, fmt.Errorf("GOKKU_DOKKU_SUDO: %q is not a boolean", *dokkuSudo))
	}
	cfg.DokkuSudo = sudo

	if cfg.Runner != RunnerExec && cfg.Runner != RunnerFake {
		errs = append(errs, fmt.Errorf("GOKKU_RUNNER: %q must be %q or %q", cfg.Runner, RunnerExec, RunnerFake))
	}

	cfg.TrustedProxies, err = parsePrefixes(*proxies)
	if err != nil {
		errs = append(errs, fmt.Errorf("GOKKU_TRUSTED_PROXIES: %w", err))
	}

	if err := cfg.LogLevel.UnmarshalText([]byte(*logLevel)); err != nil {
		errs = append(errs, fmt.Errorf("GOKKU_LOG_LEVEL: %q is not a log level", *logLevel))
	}

	return cfg, fs.Args(), errors.Join(errs...)
}

// FromOS loads the configuration from the process environment.
func FromOS(args []string) (Config, error) {
	return Load(args, os.LookupEnv)
}

// FromOSArgs is FromOS for commands with positional arguments.
func FromOSArgs(args []string) (Config, []string, error) {
	return LoadArgs(args, os.LookupEnv)
}

func parsePrefixes(s string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for part := range strings.SplitSeq(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if !strings.Contains(part, "/") {
			addr, err := netip.ParseAddr(part)
			if err != nil {
				return nil, fmt.Errorf("%q is not an address or CIDR", part)
			}
			out = append(out, netip.PrefixFrom(addr, addr.BitLen()))
			continue
		}
		p, err := netip.ParsePrefix(part)
		if err != nil {
			return nil, fmt.Errorf("%q is not an address or CIDR", part)
		}
		out = append(out, p.Masked())
	}
	return out, nil
}
