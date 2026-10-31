// Package projects applies a project's shared variables to its member apps (SPEC.md
// section 5): it reads each app's config and provenance, asks internal/reconcile what to
// change, writes through the Dokku client and stores the provenance.
package projects

import (
	"context"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"time"

	"gokku/internal/dokku"
	"gokku/internal/reconcile"
	"gokku/internal/secrets"
	"gokku/internal/store"
)

// Env reads, saves and applies shared variables.
type Env struct {
	db    *store.DB
	box   *secrets.Box
	dokku *dokku.Client
}

// NewEnv returns the shared variables service.
func NewEnv(db *store.DB, box *secrets.Box, client *dokku.Client) *Env {
	return &Env{db: db, box: box, dokku: client}
}

// Var is a decrypted shared variable.
type Var struct {
	Key       string
	Value     string
	Secret    bool
	UpdatedAt time.Time
}

// keyRe matches the project_env CHECK constraint.
var keyRe = regexp.MustCompile(`^[A-Z_][A-Z0-9_]{0,127}$`)

// ValidKey reports whether k may be a shared variable key.
func ValidKey(k string) bool { return keyRe.MatchString(k) }

// Reserved explains why a key cannot be shared, or returns "" (SHV-4): Dokku's and
// Gokku's own prefixes, PORT, and the keys the installed datastores set on link.
func Reserved(key string, datastores []string) string {
	switch {
	case strings.HasPrefix(key, "DOKKU_"), strings.HasPrefix(key, "GOKKU_"):
		return "Keys starting with DOKKU_ or GOKKU_ are reserved."
	case key == "PORT":
		return "PORT is set by Dokku."
	}
	for _, typ := range datastores {
		if dokku.LinkEnvVar(typ) == key {
			return key + " is set when a " + typ + " service is linked."
		}
	}
	return ""
}

// Vars returns the project's variables, decrypted (SHV-5).
func (e *Env) Vars(ctx context.Context, projectID string) ([]Var, error) {
	rows, err := e.db.ProjectEnv(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]Var, 0, len(rows))
	for _, r := range rows {
		v, err := e.box.Open(r.ValueEnc)
		if err != nil {
			return nil, fmt.Errorf("decrypt %s: %w", r.Key, err)
		}
		out = append(out, Var{Key: r.Key, Value: string(v), Secret: r.IsSecret, UpdatedAt: r.UpdatedAt.Time})
	}
	return out, nil
}

// Values returns the project's variables as a map.
func (e *Env) Values(ctx context.Context, projectID string) (map[string]string, error) {
	vars, err := e.Vars(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(vars))
	for _, v := range vars {
		out[v.Key] = v.Value
	}
	return out, nil
}

// Save encrypts and stores vars as the project's whole set. The caller validates them.
func (e *Env) Save(ctx context.Context, projectID string, vars []Var, by string) error {
	rows := make([]store.ProjectEnvVar, 0, len(vars))
	for _, v := range vars {
		sealed, err := e.box.Seal([]byte(v.Value))
		if err != nil {
			return err
		}
		row := store.ProjectEnvVar{ProjectID: projectID, Key: v.Key, ValueEnc: sealed, IsSecret: v.Secret}
		if by != "" {
			row.UpdatedBy = &by
		}
		rows = append(rows, row)
	}
	// Sealing uses a random nonce, so an unchanged value would look changed; keep the
	// stored ciphertext for values that did not change.
	current, err := e.db.ProjectEnv(ctx, projectID)
	if err != nil {
		return err
	}
	for i, r := range rows {
		for _, c := range current {
			if c.Key == r.Key && c.IsSecret == r.IsSecret {
				if old, err := e.box.Open(c.ValueEnc); err == nil && string(old) == vars[i].Value {
					rows[i].ValueEnc = c.ValueEnc
				}
			}
		}
	}
	return e.db.ReplaceProjectEnv(ctx, projectID, rows)
}

// Statuses reads each app's config and provenance and returns every variable's status per
// app, without writing anything (SHV-6).
func (e *Env) Statuses(ctx context.Context, projectID string, vars map[string]string, apps []string) (map[string]map[string]reconcile.Status, error) {
	out := make(map[string]map[string]reconcile.Status, len(apps))
	for _, app := range apps {
		state, err := e.appState(ctx, projectID, app)
		if err != nil {
			return nil, err
		}
		_, statuses := reconcile.Plan(vars, state)
		out[app] = statuses
	}
	return out, nil
}

func (e *Env) appState(ctx context.Context, projectID, app string) (reconcile.App, error) {
	current, err := e.dokku.Config(ctx, app)
	if err != nil {
		return reconcile.App{}, err
	}
	applied, err := e.db.AppProvenance(ctx, app, projectID)
	if err != nil {
		return reconcile.App{}, err
	}
	return reconcile.App{Current: current, Applied: applied}, nil
}

// Apply writes the project's variables into one app and records the provenance. It
// reports whether the app's config changed. log receives one line per write, with keys
// and never values.
func (e *Env) Apply(ctx context.Context, projectID, app string, vars map[string]string, log func(string)) (bool, error) {
	state, err := e.appState(ctx, projectID, app)
	if err != nil {
		return false, err
	}
	change, _ := reconcile.Plan(vars, state)
	return e.write(ctx, projectID, app, change, log)
}

// Release removes the project's managed, unchanged variables from an app leaving it
// (SHV-2). applied is the app's provenance; pass nil to read it, or the rows saved before
// the project was deleted (PRJ-4).
func (e *Env) Release(ctx context.Context, projectID, app string, applied map[string]string, log func(string)) (bool, error) {
	if applied == nil {
		var err error
		if applied, err = e.db.AppProvenance(ctx, app, projectID); err != nil {
			return false, err
		}
	}
	if len(applied) == 0 {
		return false, nil
	}
	current, err := e.dokku.Config(ctx, app)
	if err != nil {
		return false, err
	}
	change := reconcile.Release(reconcile.App{Current: current, Applied: applied})
	return e.write(ctx, projectID, app, change, log)
}

func (e *Env) write(ctx context.Context, projectID, app string, c reconcile.Change, log func(string)) (bool, error) {
	if len(c.Set) > 0 {
		if err := e.dokku.SetConfig(ctx, app, c.Set); err != nil {
			return false, err
		}
		log(fmt.Sprintf("%s: set %s", app, strings.Join(slices.Sorted(maps.Keys(c.Set)), ", ")))
	}
	if len(c.Unset) > 0 {
		if err := e.dokku.UnsetConfig(ctx, app, c.Unset); err != nil {
			return false, err
		}
		log(fmt.Sprintf("%s: unset %s", app, strings.Join(c.Unset, ", ")))
	}
	if !c.Writes() {
		log(app + ": up to date")
	}
	if err := e.db.UpdateProvenance(ctx, app, projectID, c.Record, c.Forget); err != nil {
		return c.Writes(), err
	}
	return c.Writes(), nil
}
