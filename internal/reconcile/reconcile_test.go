package reconcile

import (
	"maps"
	"slices"
	"testing"
)

func app(current map[string]string, applied map[string]string) App {
	if current == nil {
		current = map[string]string{}
	}
	if applied == nil {
		applied = map[string]string{}
	}
	return App{Current: current, Applied: applied}
}

// One test per row of the table in SPEC.md section 5.
func TestSHVTableRows(t *testing.T) {
	vars := map[string]string{"LOG_LEVEL": "info"}
	for _, c := range []struct {
		name   string
		app    App
		set    map[string]string
		record map[string]string
		forget []string
		status Status
	}{
		{
			name: "row 1: the app has no value: write it and record the hash",
			app:  app(nil, nil),
			set:  vars, record: map[string]string{"LOG_LEVEL": Hash("info")}, status: Pending,
		},
		{
			name: "row 1, again: the app removed a value Gokku wrote: write it back",
			app:  app(nil, map[string]string{"LOG_LEVEL": Hash("info")}),
			set:  vars, record: map[string]string{"LOG_LEVEL": Hash("info")}, status: Pending,
		},
		{
			name:   "row 2: managed and unchanged, same value: nothing to do",
			app:    app(map[string]string{"LOG_LEVEL": "info"}, map[string]string{"LOG_LEVEL": Hash("info")}),
			status: Applied,
		},
		{
			name: "row 2: managed and unchanged, the project's value changed: write and update",
			app:  app(map[string]string{"LOG_LEVEL": "debug"}, map[string]string{"LOG_LEVEL": Hash("debug")}),
			set:  vars, record: map[string]string{"LOG_LEVEL": Hash("info")}, status: Pending,
		},
		{
			name:   "row 3: managed, but the app changed it: drop the row, do not write",
			app:    app(map[string]string{"LOG_LEVEL": "warn"}, map[string]string{"LOG_LEVEL": Hash("debug")}),
			forget: []string{"LOG_LEVEL"}, status: Overridden,
		},
		{
			name:   "row 4: not managed, same value: adopt without writing",
			app:    app(map[string]string{"LOG_LEVEL": "info"}, nil),
			record: map[string]string{"LOG_LEVEL": Hash("info")}, status: Applied,
		},
		{
			name:   "row 5: not managed, a different value: leave it",
			app:    app(map[string]string{"LOG_LEVEL": "warn"}, nil),
			status: Overridden,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			change, statuses := Plan(vars, c.app)
			if !maps.Equal(change.Set, nonNil(c.set)) || !maps.Equal(change.Record, nonNil(c.record)) ||
				!slices.Equal(change.Forget, c.forget) || len(change.Unset) != 0 {
				t.Errorf("change = %+v", change)
			}
			if statuses["LOG_LEVEL"] != c.status {
				t.Errorf("status = %s, want %s", statuses["LOG_LEVEL"], c.status)
			}
			if change.Writes() != (len(c.set) > 0) {
				t.Errorf("Writes() = %v", change.Writes())
			}
		})
	}
}

func nonNil(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

// SHV-1: removing a variable unsets it only where it is managed and unchanged.
func TestSHV1RemovedVariable(t *testing.T) {
	applied := map[string]string{"OLD": Hash("x"), "CHANGED": Hash("y")}
	current := map[string]string{"OLD": "x", "CHANGED": "edited", "OTHER": "z"}
	change, statuses := Plan(map[string]string{}, app(current, applied))
	if !slices.Equal(change.Unset, []string{"OLD"}) || !slices.Equal(change.Forget, []string{"CHANGED", "OLD"}) {
		t.Errorf("change = %+v", change)
	}
	if len(statuses) != 0 || !change.Writes() {
		t.Errorf("statuses = %v", statuses)
	}
}

// SHV-2, PRJ-4: an app that leaves the project loses only the managed, unchanged keys.
func TestSHV2Release(t *testing.T) {
	a := app(
		map[string]string{"LOG_LEVEL": "info", "TZ": "Europe/Paris", "OWN": "1"},
		map[string]string{"LOG_LEVEL": Hash("info"), "TZ": Hash("UTC"), "GONE": Hash("x")},
	)
	change := Release(a)
	if !slices.Equal(change.Unset, []string{"LOG_LEVEL"}) || !slices.Equal(change.Forget, []string{"GONE", "LOG_LEVEL", "TZ"}) ||
		len(change.Set) != 0 {
		t.Errorf("change = %+v", change)
	}
	if Release(app(map[string]string{"A": "1"}, nil)).Writes() {
		t.Error("an app with nothing managed must not be written")
	}
}

// Moving an app is a release from the old project, then a plan in the new one.
func TestMoveBetweenProjects(t *testing.T) {
	current := map[string]string{"LOG_LEVEL": "info"}
	out := Release(app(current, map[string]string{"LOG_LEVEL": Hash("info")}))
	after := maps.Clone(current)
	for _, k := range out.Unset {
		delete(after, k)
	}
	change, statuses := Plan(map[string]string{"LOG_LEVEL": "debug"}, app(after, nil))
	if change.Set["LOG_LEVEL"] != "debug" || statuses["LOG_LEVEL"] != Pending {
		t.Errorf("after the move: %+v %v", change, statuses)
	}
}

func TestHash(t *testing.T) {
	if Hash("") != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Error("Hash is not sha256 hex")
	}
}
