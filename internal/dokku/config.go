package dokku

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
)

// configKeyRe is Dokku's rule for config keys (plugins/config/config.go, validateKey).
var configKeyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,254}$`)

// ValidConfigKey reports whether k is a valid config key.
func ValidConfigKey(k string) bool { return configKeyRe.MatchString(k) }

// Config returns the app's environment from `dokku config:export --format json <app>`.
func (cl *Client) Config(ctx context.Context, app string) (map[string]string, error) {
	if !ValidAppName(app) {
		return nil, fmt.Errorf("%w: app %q", ErrInvalidName, app)
	}
	out, err := cl.run(ctx, newCmd("config:export", "--format", "json", app))
	if err != nil {
		return nil, err
	}
	return parseConfigJSON(out)
}

func parseConfigJSON(out string) (map[string]string, error) {
	env := map[string]string{}
	if strings.TrimSpace(out) == "" {
		return env, nil
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		return nil, fmt.Errorf("config:export: %w", err)
	}
	return env, nil
}

// SetConfig sets variables without restarting the app. The values go to
// `dokku config:import --no-restart --format json <app> -` on stdin, so they never appear in
// process arguments (CFG-4). Its output, which repeats the values, is discarded.
func (cl *Client) SetConfig(ctx context.Context, app string, vars map[string]string) error {
	if len(vars) == 0 {
		return nil
	}
	if !ValidAppName(app) {
		return fmt.Errorf("%w: app %q", ErrInvalidName, app)
	}
	c := newCmd("config:import", "--no-restart", "--format", "json", app, "-")
	for _, k := range slices.Sorted(maps.Keys(vars)) {
		if !ValidConfigKey(k) {
			return fmt.Errorf("%w: config key %q", ErrInvalidName, k)
		}
		c = c.secretValue(vars[k])
	}
	body, err := json.Marshal(vars)
	if err != nil {
		return err
	}
	var stderr []string
	err = cl.stream(ctx, c, bytes.NewReader(body), func(l Line) {
		if l.Stream == "stderr" {
			stderr = append(stderr, l.Text)
		}
	})
	if exitErr, ok := errors.AsType[*ExitError](err); ok && exitErr.Stderr == "" {
		exitErr.Stderr = c.scrub(strings.Join(stderr, "\n"))
	}
	return err
}

// UnsetConfig removes keys without restarting the app.
func (cl *Client) UnsetConfig(ctx context.Context, app string, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	if !ValidAppName(app) {
		return fmt.Errorf("%w: app %q", ErrInvalidName, app)
	}
	args := []string{"config:unset", "--no-restart", app}
	for _, k := range keys {
		if !ValidConfigKey(k) {
			return fmt.Errorf("%w: config key %q", ErrInvalidName, k)
		}
		args = append(args, k)
	}
	_, err := cl.run(ctx, newCmd(args...))
	return err
}

// ServiceDSN returns a service's connection string from `<type>:info <name> --dsn`. It is
// a secret: callers compare it and never log or return it.
func (cl *Client) ServiceDSN(ctx context.Context, typ, name string) (string, error) {
	if !IsDatastore(typ) || !ValidServiceName(name) {
		return "", fmt.Errorf("%w: service %s/%s", ErrInvalidName, typ, name)
	}
	out, err := cl.run(ctx, newCmd(typ+":info", name, "--dsn"))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}
