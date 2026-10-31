// Package reconcile decides how a project's shared variables are written into its member
// apps (SPEC.md section 5). It does no I/O: the caller reads each app's config and
// provenance, applies the Change through the runner and stores the provenance updates in
// one transaction per app (DESIGN.md 3.6).
package reconcile

import (
	"crypto/sha256"
	"encoding/hex"
	"maps"
	"slices"
)

// Status is how a shared variable stands on one app (SHV-7).
type Status string

// Statuses.
const (
	Applied    Status = "applied"    // the app holds the project's value, written or adopted by Gokku
	Overridden Status = "overridden" // the app sets its own value, which Gokku leaves alone
	Pending    Status = "pending"    // the next apply writes the project's value
)

// App is one member app as it is now.
type App struct {
	Current map[string]string // the app's config
	Applied map[string]string // provenance: key -> sha256 of the value the project wrote
}

// Change is what applying does to one app.
type Change struct {
	Set    map[string]string // config to write
	Unset  []string          // config keys to remove
	Record map[string]string // provenance rows to store: key -> hash
	Forget []string          // provenance rows to delete
}

// Writes reports whether the change touches the app's config, so it needs a restart to
// take effect (SHV-3).
func (c Change) Writes() bool { return len(c.Set) > 0 || len(c.Unset) > 0 }

// Hash is the provenance hash of a value: sha256, hex.
func Hash(v string) string {
	sum := sha256.Sum256([]byte(v))
	return hex.EncodeToString(sum[:])
}

func newChange() Change {
	return Change{Set: map[string]string{}, Record: map[string]string{}}
}

// Plan applies the project's variables to one app, row by row of the SPEC section 5 table,
// and removes the keys the project no longer has (SHV-1). It returns the change and each
// variable's status as it stands now, before the change (SHV-6: drift is computed on read).
func Plan(vars map[string]string, app App) (Change, map[string]Status) {
	c := newChange()
	statuses := make(map[string]Status, len(vars))
	for _, k := range slices.Sorted(maps.Keys(vars)) {
		v := vars[k]
		cur, has := app.Current[k]
		prov, managed := app.Applied[k]
		switch {
		case !has:
			// The app has no value: write it.
			c.Set[k], c.Record[k], statuses[k] = v, Hash(v), Pending
		case managed && Hash(cur) == prov:
			// Gokku's value is untouched: update it if the project's value changed.
			if Hash(v) != prov {
				c.Set[k], c.Record[k], statuses[k] = v, Hash(v), Pending
			} else {
				statuses[k] = Applied
			}
		case managed:
			// The app changed a value Gokku wrote: it becomes the app's own.
			c.Forget = append(c.Forget, k)
			statuses[k] = Overridden
		case cur == v:
			// The app already holds the project's value: adopt it without writing.
			c.Record[k], statuses[k] = Hash(v), Applied
		default:
			statuses[k] = Overridden
		}
	}
	for _, k := range slices.Sorted(maps.Keys(app.Applied)) {
		if _, kept := vars[k]; !kept {
			removeKey(&c, app, k)
		}
	}
	return c, statuses
}

// Release removes everything the project manages from an app that leaves it, or whose
// project is deleted (SHV-2, PRJ-4): managed, unchanged keys are unset, and every
// provenance row goes.
func Release(app App) Change {
	c := newChange()
	for _, k := range slices.Sorted(maps.Keys(app.Applied)) {
		removeKey(&c, app, k)
	}
	return c
}

// removeKey unsets k where it is still managed and unchanged; copies the app changed are
// left alone (SHV-1). The provenance row goes either way.
func removeKey(c *Change, app App, k string) {
	if cur, has := app.Current[k]; has && Hash(cur) == app.Applied[k] {
		c.Unset = append(c.Unset, k)
	}
	c.Forget = append(c.Forget, k)
}
