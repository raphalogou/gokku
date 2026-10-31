package dokku

import (
	"errors"
	"slices"
	"testing"
	"time"
)

// APP-3, APP-4, APP-5 on the fake runner.
func TestManageAppsOnFake(t *testing.T) {
	fake := NewFakeRunner(time.Now())
	cl := NewClient(fake, discard())
	ctx := t.Context()
	if err := cl.CreateApp(ctx, "new-app"); err != nil {
		t.Fatal(err)
	}
	if err := cl.CreateApp(ctx, "new-app"); err == nil {
		t.Error("a duplicate name was created")
	}
	if err := cl.RenameApp(ctx, "blog-api", "api", func(Line) {}); err != nil {
		t.Fatal(err)
	}
	services, _ := cl.Services(ctx, "postgres")
	if !slices.Contains(services[0].Links, "api") {
		t.Errorf("links after rename = %v", services[0].Links)
	}
	if err := cl.DestroyApp(ctx, "api", func(Line) {}); err != nil {
		t.Fatal(err)
	}
	reports, _ := cl.AppReports(ctx)
	if _, ok := reports["api"]; ok || reports["new-app"] == nil {
		t.Errorf("apps = %v", reports)
	}
	if !slices.Contains(fake.Calls(), "apps:destroy --force api") {
		t.Errorf("calls = %v", fake.Calls())
	}
	for _, bad := range []string{"-x", "Bad", ""} {
		if err := cl.CreateApp(ctx, bad); !errors.Is(err, ErrInvalidName) {
			t.Errorf("CreateApp(%q) = %v", bad, err)
		}
	}
}
