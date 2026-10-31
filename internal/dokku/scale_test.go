package dokku

import (
	"errors"
	"maps"
	"testing"
	"time"
)

func TestParseScaleFixture(t *testing.T) {
	got, err := parseScale(fixture(t, "ps-scale.json"))
	if err != nil || !maps.Equal(got, map[string]int{"web": 1, "worker": 4}) {
		t.Errorf("scale = %v, %v", got, err)
	}
	if got, err := parseScale("[]"); err != nil || len(got) != 0 {
		t.Errorf("empty = %v, %v", got, err)
	}
}

// SCL-1 on the fake runner.
func TestScaleOnFake(t *testing.T) {
	cl := NewClient(NewFakeRunner(time.Now()), discard())
	ctx := t.Context()
	got, err := cl.Scale(ctx, "blog-api")
	if err != nil || !maps.Equal(got, map[string]int{"web": 2, "worker": 1}) {
		t.Fatalf("scale = %v, %v", got, err)
	}
	if err := cl.SetScale(ctx, "blog-api", map[string]int{"worker": 3}, func(Line) {}); err != nil {
		t.Fatal(err)
	}
	reports, _ := cl.AppReports(ctx)
	if n := len(reports["blog-api"].Containers); n != 5 || !reports["blog-api"].CanScale {
		t.Errorf("containers = %d, can scale %v", n, reports["blog-api"].CanScale)
	}
	for _, bad := range []map[string]int{{"-web": 1}, {"web": -1}, {"web": MaxScale + 1}, {"web=1 x": 1}} {
		if err := cl.SetScale(ctx, "blog-api", bad, func(Line) {}); !errors.Is(err, ErrInvalidName) {
			t.Errorf("%v accepted: %v", bad, err)
		}
	}
}
