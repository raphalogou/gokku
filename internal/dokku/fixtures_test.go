package dokku

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// fixtureVersion is the Dokku version the golden fixtures were taken from.
const fixtureVersion = "0.38.34"

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", fixtureVersion, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestParseVersionFixture(t *testing.T) {
	v, err := parseVersion(fixture(t, "version.txt"))
	if err != nil || v != "0.38.34" {
		t.Errorf("version = %q, %v", v, err)
	}
	if _, err := parseVersion("sudo: a password is required"); err == nil {
		t.Error("unexpected output must be an error")
	}
}

func TestParsePluginListFixtures(t *testing.T) {
	core, err := parsePluginList(fixture(t, "plugin-list.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if len(core) != 29 || core[0] != (Plugin{Name: "00_dokku-standard", Version: "0.38.34", Enabled: true,
		Description: "dokku core standard plugin"}) {
		t.Errorf("core plugins = %d, first %+v", len(core), core[0])
	}
	i := slices.IndexFunc(core, func(p Plugin) bool { return p.Name == "scheduler-docker-local" })
	if i < 0 || core[i].Version != "0.38.34" {
		t.Errorf("a name longer than its column must still parse: %+v", core)
	}

	all, err := parsePluginList(fixture(t, "plugin-list-third-party.txt"))
	if err != nil {
		t.Fatal(err)
	}
	c := capabilities("0.38.34", all)
	if !slices.Equal(c.Datastores, []string{"postgres"}) || !c.Letsencrypt ||
		c.Unavailable["redis"] == "" || c.Unavailable["mysql"] != "The plugin is disabled." {
		t.Errorf("capabilities = %+v", c)
	}

	if _, err := parsePluginList("plugn: dev\n  broken line\n"); err == nil {
		t.Error("a malformed line must be an error")
	}
}

func TestParseAppReportFixtures(t *testing.T) {
	apps, err := parseReport(fixture(t, "apps-report.txt"), "app")
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 3 || apps["node-js-app"]["app-deploy-source-metadata"] != "cd7b8afccb202f222e7dc7b427553e71ba5ddafd" ||
		apps["python-sample"]["app-deploy-source"] != "" || apps["ruby-sample"]["app-locked"] != "false" {
		t.Errorf("apps:report = %v", apps)
	}

	domains, err := parseReport(fixture(t, "domains-report.txt"), "domains")
	if err != nil {
		t.Fatal(err)
	}
	if got := domains["node-js-app"].list("domains-app-vhosts", " "); !slices.Equal(got, []string{"node-js-app.dokku.org"}) {
		t.Errorf("vhosts = %v", got)
	}

	git, err := parseReport(fixture(t, "git-report.txt"), "git")
	if err != nil {
		t.Fatal(err)
	}
	if g := git["node-js-app"]; g["git-sha"] != "a1b2c3d" || !unixTime(g["git-last-updated-at"]).Equal(time.Unix(1700000000, 0)) {
		t.Errorf("git:report = %v", g)
	}

	builder, err := parseReport(fixture(t, "builder-report.txt"), "builder")
	if err != nil {
		t.Fatal(err)
	}
	if builder["python-sample"]["builder-computed-selected"] != "dockerfile" || builder["ruby-sample"]["builder-selected"] != "" {
		t.Errorf("builder:report = %v", builder)
	}

	// The wrong kind of report is refused rather than misread.
	if _, err := parseReport(fixture(t, "git-report.txt"), "ps"); err == nil {
		t.Error("a git report parsed as a ps report")
	}
}

func TestParsePsReportFixtures(t *testing.T) {
	undeployed, err := parseReport(fixture(t, "ps-report.txt"), "ps")
	if err != nil {
		t.Fatal(err)
	}
	if p := undeployed["node-js-app"]; len(undeployed) != 3 || p.bool("deployed") || p["running"] != "false" ||
		p["ps-procfile-path"] != "Procfile2" || len(parseContainers(p)) != 0 {
		t.Errorf("ps:report = %v", undeployed)
	}

	deployed, err := parseReport(fixture(t, "ps-report-deployed.txt"), "ps")
	if err != nil {
		t.Fatal(err)
	}
	api := deployed["blog-api"]
	want := []Container{{"web", 1, "running"}, {"web", 2, "running"}, {"worker", 1, "running"}}
	if got := parseContainers(api); !slices.Equal(got, want) || api["running"] != "true" {
		t.Errorf("blog-api containers = %v", got)
	}
	staging := deployed["staging-web"]
	if got := parseContainers(staging); !slices.Equal(got, []Container{{"web", 1, "exited"}}) || staging.bool("restore") {
		t.Errorf("staging-web = %v", staging)
	}
	if _, ok := deployed["api.v2"]; !ok {
		t.Error("an app name with a dot was dropped")
	}
}

func TestParseServicesFixture(t *testing.T) {
	services, err := parseServices(fixture(t, "redis-info.txt"), "redis")
	if err != nil {
		t.Fatal(err)
	}
	want := []Service{
		{Type: "redis", Name: "blog-cache", Status: "running", Version: "redis:7.4-alpine", Links: []string{"blog-api", "worker-queue"}},
		{Type: "redis", Name: "spare_Cache", Status: "exited", Version: "redis:7.4-alpine"},
	}
	if len(services) != 2 {
		t.Fatalf("services = %+v", services)
	}
	for i, w := range want {
		g := services[i]
		if g.Name != w.Name || g.Status != w.Status || g.Version != w.Version || !slices.Equal(g.Links, w.Links) {
			t.Errorf("service %d = %+v, want %+v", i, g, w)
		}
	}
	// The report carries the connection string; nothing of it survives parsing (SEC-5).
	if strings.Contains(fmt.Sprintf("%+v", services), "not-a-real-password") {
		t.Error("the DSN leaked into the parsed services")
	}
}

// fixtureCommands maps the commands the snapshot runs to the fixtures that answer them.
var fixtureCommands = map[string]string{
	"version":        "version.txt",
	"plugin:list":    "plugin-list-third-party.txt",
	"apps:report":    "apps-report.txt",
	"ps:report":      "ps-report.txt",
	"domains:report": "domains-report.txt",
	"git:report":     "git-report.txt",
	"builder:report": "builder-report.txt",
	"postgres:info":  "redis-info.txt", // no postgres sample; the format is shared and the kind is checked
}

// helperRunner runs the test binary as a stand-in "dokku" that prints the fixtures, so
// the exec runner, the client and the parsers run together as they do on a server.
type helperRunner struct{ *ExecRunner }

func (h helperRunner) Run(ctx context.Context, args ...string) (Result, error) {
	return h.ExecRunner.Run(ctx, append([]string{"-test.run=^TestHelperProcess$", "--", "fixture"}, args...)...)
}

func TestFixturesThroughExecRunner(t *testing.T) {
	cl := NewClient(helperRunner{&ExecRunner{Bin: os.Args[0]}}, discard())
	caps, err := cl.Probe(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if caps.DokkuVersion != "0.38.34" || !slices.Equal(caps.Datastores, []string{"postgres"}) {
		t.Errorf("probe = %+v", caps)
	}
	reports, err := cl.AppReports(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	node := reports["node-js-app"]
	if len(reports) != 3 || node == nil || node.DeploySource != "git" || node.GitSHA != "a1b2c3d" ||
		node.Builder != "herokuish" || !slices.Equal(node.Domains, []string{"node-js-app.dokku.org"}) {
		t.Errorf("node-js-app = %+v", node)
	}
	// The docs samples name the domains report's apps differently; those are not invented.
	if reports["python-sample"].Domains != nil {
		t.Errorf("python-sample = %+v", reports["python-sample"])
	}
	// A datastore whose report has the wrong kind is an error, not an empty list.
	if _, err := cl.Services(t.Context(), "postgres"); err == nil {
		t.Error("a redis report was accepted for postgres")
	}
	if _, err := cl.run(t.Context(), newCmd("apps:destroy", "x")); err == nil {
		t.Error("the stand-in answers only the recorded commands")
	}
}
