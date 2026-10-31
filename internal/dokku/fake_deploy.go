package dokku

import (
	"crypto/sha1" //nolint:gosec // fake commit ids, not security
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// deployCommand answers git:from-image and git:sync --build. An image or repository whose
// name contains "broken" fails the build, for tests of failed deploys. The caller holds f.mu.
func (f *FakeRunner) deployCommand(args []string) (script, bool) {
	var app, source, meta, fetch string
	switch {
	case len(args) == 3 && args[0] == "git:from-image":
		app, source, meta = args[1], "docker-image", args[2]
		fetch = "-----> Pulling image"
	case (len(args) == 4 || len(args) == 5) && args[0] == "git:sync" && args[1] == "--build":
		ref := "main"
		if len(args) == 5 {
			ref = args[4]
		}
		app, source, meta = args[2], "git-sync", args[3]+"#"+ref
		fetch = "-----> Cloning " + app + " from " + args[3] + "#" + ref
	default:
		return script{}, false
	}
	a, ok := f.apps[app]
	if !ok {
		return failScript(fmt.Sprintf("App %s does not exist", app)), true
	}
	builder := "dockerfile"
	if source == "git-sync" {
		builder = "herokuish"
	}
	lines := []string{fetch, "-----> Generating build context", "-----> Cleaning up...",
		"-----> Building " + app + " from " + builder, "       Step 1/3 : FROM base", "       Step 2/3 : COPY . /app",
		"       Step 3/3 : CMD start"}
	if strings.Contains(meta, "broken") {
		ls := out(lines...)
		ls = append(ls, Line{Stream: "stderr", Text: " !     Build failed: step 2/3 exited with code 1"})
		return script{lines: ls, exit: 1}, true
	}
	sum := sha1.Sum([]byte(meta + time.Now().String())) //nolint:gosec // see import
	a.sha = hex.EncodeToString(sum[:])
	a.deploySource, a.deployMeta, a.builder = source, meta, builder
	if source == "docker-image" {
		a.sourceImage = meta
	}
	a.updatedAt = time.Now().Truncate(time.Second)
	a.deployed, a.restore = true, true
	if len(a.processes) == 0 {
		a.processes = map[string]int{"web": 1}
	}
	lines = append(lines, "=====> Processing deployment checks", "-----> Deploying web (count=1)",
		"       Attempting pre-flight checks (web.1)", "       Default container check successful (web.1)",
		"-----> Releasing "+app+"...", "=====> Application deployed:")
	for _, d := range a.domains {
		lines = append(lines, "       http://"+d)
	}
	return script{lines: out(lines...)}, true
}
