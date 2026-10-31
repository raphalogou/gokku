package dokku

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var (
	// imageRe is a Docker image reference: an optional registry host (with port), a
	// lowercase path, then an optional tag and digest.
	imageRe = regexp.MustCompile(`^([a-zA-Z0-9][a-zA-Z0-9.-]*(:[0-9]{1,5})?/)?[a-z0-9]+([._-][a-z0-9]+)*(/[a-z0-9]+([._-][a-z0-9]+)*)*(:[A-Za-z0-9_][A-Za-z0-9_.-]{0,127})?(@sha256:[a-f0-9]{64})?$`)
	// scpRe is git's scp-like syntax: user@host:path.
	scpRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+@[A-Za-z0-9.-]+:[A-Za-z0-9_./~-]+$`)
	// refRe is a branch, tag or commit; ".." and a leading dash are refused separately.
	refRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_./-]{0,254}$`)
)

// ValidImage reports whether ref is a Docker image reference (DEP-1).
func ValidImage(ref string) bool { return len(ref) <= 255 && imageRe.MatchString(ref) }

// ValidRepositoryURL reports whether u is a remote git repository: https, http, ssh or
// git URLs, or user@host:path. file: URLs, local paths and URLs with a password are
// refused (DEP-1).
func ValidRepositoryURL(u string) bool {
	if len(u) > 2048 || strings.HasPrefix(u, "-") || strings.ContainsAny(u, " \t\n") {
		return false
	}
	if scpRe.MatchString(u) {
		return true
	}
	p, err := url.Parse(u)
	if err != nil || p.Host == "" {
		return false
	}
	if _, hasPassword := p.User.Password(); hasPassword {
		return false
	}
	switch p.Scheme {
	case "https", "http", "ssh", "git":
		return true
	}
	return false
}

// ValidRef reports whether ref is a usable branch, tag or commit.
func ValidRef(ref string) bool {
	return refRe.MatchString(ref) && !strings.Contains(ref, "..") && !strings.HasSuffix(ref, "/")
}

// DeployImage runs `dokku git:from-image <app> <image>`, which builds and deploys the image,
// streaming the output.
func (cl *Client) DeployImage(ctx context.Context, app, image string, onLine func(Line)) error {
	if !ValidAppName(app) || !ValidImage(image) {
		return fmt.Errorf("%w: app %q or image %q", ErrInvalidName, app, image)
	}
	return cl.stream(ctx, newCmd("git:from-image", app, image), nil, onLine)
}

// DeployRepository runs `dokku git:sync --build <app> <url> [<ref>]`, streaming the output.
// URLs with credentials are refused; private repositories use `dokku git:auth`.
func (cl *Client) DeployRepository(ctx context.Context, app, repo, ref string, onLine func(Line)) error {
	if !ValidAppName(app) || !ValidRepositoryURL(repo) || (ref != "" && !ValidRef(ref)) {
		return fmt.Errorf("%w: app %q, repository or ref %q", ErrInvalidName, app, ref)
	}
	args := []string{"git:sync", "--build", app, repo}
	if ref != "" {
		args = append(args, ref)
	}
	return cl.stream(ctx, newCmd(args...), nil, onLine)
}

// Deploy steps, in the order Dokku runs them (DEP-2).
var DeploySteps = []string{"Fetch source", "Detect builder", "Build image", "Run checks", "Release"}

// deployMarkers map the start of Dokku's own log lines to the step they begin
// (plugins/git, plugins/common, plugins/builder-*, plugins/scheduler-docker-local).
var deployMarkers = []struct {
	prefix string
	step   int
}{
	{"-----> Cloning ", 0}, {"-----> Pulling image", 0}, {"-----> Generating build context", 0},
	{"-----> Updating git repository", 0}, {"-----> Fetching ", 0}, {"-----> Initializing git repository", 0},
	{"-----> Cleaning up...", 1}, {"-----> Adding BUILD_ENV", 1},
	{"-----> Building ", 2},
	{"=====> Processing deployment checks", 3}, {"-----> Running pre-flight checks", 3}, {"-----> Deploying ", 3},
	{"-----> Releasing ", 4}, {"=====> Application deployed", 4},
}

// DeployStep returns the step a line of deploy output begins, or -1.
func DeployStep(line string) int {
	for _, m := range deployMarkers {
		if strings.HasPrefix(line, m.prefix) {
			return m.step
		}
	}
	return -1
}
