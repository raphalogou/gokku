package dokku

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

// DEP-1: what the deploy sources accept.
func TestDEP1Validators(t *testing.T) {
	for ref, ok := range map[string]bool{
		"nginx": true, "nginx:1.28-alpine": true, "ghcr.io/acme/app:v2": true, "registry.local:5000/team/app": true,
		"library/redis@sha256:" + strings.Repeat("a", 64): true,
		"": false, "-v": false, "Nginx": false, "nginx:": false, "a b": false, "../x": false, "x;rm": false,
	} {
		if ValidImage(ref) != ok {
			t.Errorf("ValidImage(%q) = %v", ref, !ok)
		}
	}
	for u, ok := range map[string]bool{
		"https://github.com/acme/app.git": true, "ssh://git@github.com/acme/app.git": true, "git@github.com:acme/app.git": true,
		"http://git.local/app": true, "git://host/repo": true,
		"file:///etc/passwd": false, "/var/lib/repo": false, "./repo": false, "--upload-pack=x": false,
		"https://user:token@github.com/acme/app.git": false, "ftp://host/x": false, "https:///x": false, "a b": false,
	} {
		if ValidRepositoryURL(u) != ok {
			t.Errorf("ValidRepositoryURL(%q) = %v", u, !ok)
		}
	}
	for r, ok := range map[string]bool{"main": true, "v1.2.3": true, "feature/x": true, "97e6c72": true,
		"-b": false, "a..b": false, "a b": false, "x/": false} {
		if ValidRef(r) != ok {
			t.Errorf("ValidRef(%q) = %v", r, !ok)
		}
	}
}

// DEP-2: Dokku's log lines mark the steps.
func TestDeployStep(t *testing.T) {
	for line, step := range map[string]int{
		"-----> Cloning app from https://x#main": 0, "-----> Pulling image": 0, "-----> Cleaning up...": 1,
		"-----> Building app from herokuish": 2, "=====> Processing deployment checks": 3,
		"-----> Releasing app...": 4, "=====> Application deployed:": 4, "       Step 1/3": -1, "Building": -1,
	} {
		if got := DeployStep(line); got != step {
			t.Errorf("DeployStep(%q) = %d, want %d", line, got, step)
		}
	}
}

func TestDeployOnFake(t *testing.T) {
	fake := NewFakeRunner(time.Now())
	cl := NewClient(fake, discard())
	ctx := t.Context()
	var steps []int
	err := cl.DeployImage(ctx, "docs-site", "nginx:1.28-alpine", func(l Line) {
		if s := DeployStep(l.Text); s >= 0 && (len(steps) == 0 || steps[len(steps)-1] != s) {
			steps = append(steps, s)
		}
	})
	if err != nil || !slices.Equal(steps, []int{0, 1, 2, 3, 4}) {
		t.Fatalf("deploy: %v, steps %v", err, steps)
	}
	reports, _ := cl.AppReports(ctx)
	if r := reports["docs-site"]; r.SourceImage != "nginx:1.28-alpine" || r.DeploySource != "docker-image" || r.GitSHA == "" {
		t.Errorf("after deploy = %+v", r)
	}
	if err := cl.DeployRepository(ctx, "docs-site", "https://github.com/acme/broken.git", "main", func(Line) {}); err == nil {
		t.Error("a broken build succeeded")
	}
	if !slices.Contains(fake.Calls(), "git:sync --build docs-site https://github.com/acme/broken.git main") {
		t.Errorf("calls = %v", fake.Calls())
	}
	if err := cl.DeployRepository(ctx, "docs-site", "file:///etc", "", func(Line) {}); !errors.Is(err, ErrInvalidName) {
		t.Errorf("file URL: %v", err)
	}
}
