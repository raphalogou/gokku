package dokku

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// AppReport is what the bulk reports say about one app.
type AppReport struct {
	Name string
	// apps:report
	CreatedAt            time.Time
	DeploySource         string // git-push, docker-image, git-sync, tar, ... (empty before the first deploy)
	DeploySourceMetadata string // commit, image reference or remote#ref
	Locked               bool   // a deploy holds the app lock
	// ps:report
	Deployed   bool
	Running    string // "true", "false" or "mixed"
	Restore    bool   // false after ps:stop
	CanScale   bool   // false when app.json sets the formation
	Containers []Container
	// domains:report
	DomainsEnabled bool
	Domains        []string
	// git:report
	GitSHA       string
	GitUpdatedAt time.Time // when the deploy branch last moved
	SourceImage  string
	// builder:report
	Builder string // the selected builder, else the detected one
}

// Container is one process container from ps:report, such as web.1.
type Container struct {
	Type  string
	Index int
	State string // docker state: running, exited, paused, restarting, missing, ...
}

// appReportKinds are the bulk reports that make an AppReport. Each prints every app.
var appReportKinds = []struct{ command, kind string }{
	{"apps:report", "app"},
	{"ps:report", "ps"},
	{"domains:report", "domains"},
	{"git:report", "git"},
	{"builder:report", "builder"},
}

// AppReports runs the bulk report commands, one per plugin and never one per app
// (NFR-2), and merges them by app name.
func (cl *Client) AppReports(ctx context.Context) (map[string]*AppReport, error) {
	results := make([]map[string]Fields, len(appReportKinds))
	errs := make([]error, len(appReportKinds))
	var wg sync.WaitGroup
	for i, k := range appReportKinds {
		wg.Go(func() {
			out, err := cl.run(ctx, newCmd(k.command))
			if err == nil {
				results[i], err = parseReport(out, k.kind)
			}
			errs[i] = err
		})
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return mergeAppReports(results[0], results[1], results[2], results[3], results[4]), nil
}

func mergeAppReports(apps, ps, domains, git, builder map[string]Fields) map[string]*AppReport {
	out := make(map[string]*AppReport, len(apps))
	for name, f := range apps {
		if !ValidAppName(name) {
			continue
		}
		r := &AppReport{
			Name:                 name,
			CreatedAt:            unixTime(f["app-created-at"]),
			DeploySource:         f["app-deploy-source"],
			DeploySourceMetadata: f["app-deploy-source-metadata"],
			Locked:               f.bool("app-locked"),
		}
		if p := ps[name]; p != nil {
			r.Deployed = p.bool("deployed")
			r.Running = p["running"]
			r.Restore = p.bool("restore")
			r.CanScale = p.bool("ps-can-scale")
			r.Containers = parseContainers(p)
		}
		if d := domains[name]; d != nil {
			r.DomainsEnabled = d.bool("domains-app-enabled")
			r.Domains = d.list("domains-app-vhosts", " ")
		}
		if g := git[name]; g != nil {
			r.GitSHA = g["git-sha"]
			r.GitUpdatedAt = unixTime(g["git-last-updated-at"])
			r.SourceImage = g["git-source-image"]
		}
		if b := builder[name]; b != nil {
			r.Builder = cmp.Or(b["builder-computed-selected"], b["builder-detected"])
		}
		out[name] = r
	}
	return out
}

// parseContainers reads the "Status <type> <n>: <state> (CID: ...)" fields of ps:report.
func parseContainers(f Fields) []Container {
	var out []Container
	for key, value := range f {
		rest, ok := strings.CutPrefix(key, "status-")
		if !ok {
			continue
		}
		i := strings.LastIndexByte(rest, '-')
		if i <= 0 {
			continue
		}
		n, err := strconv.Atoi(rest[i+1:])
		if err != nil {
			continue
		}
		state, _, _ := strings.Cut(value, " ")
		out = append(out, Container{Type: rest[:i], Index: n, State: state})
	}
	slices.SortFunc(out, func(a, b Container) int {
		if c := strings.Compare(a.Type, b.Type); c != 0 {
			return c
		}
		return a.Index - b.Index
	})
	return out
}

func unixTime(s string) time.Time {
	// apps:report can hold several comma-separated values after a rename; the first wins.
	s, _, _ = strings.Cut(s, ",")
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil || n <= 0 {
		return time.Time{}
	}
	return time.Unix(n, 0).UTC()
}

// Version returns the Dokku version, from `dokku version` ("dokku version 0.38.34").
func (cl *Client) Version(ctx context.Context) (string, error) {
	out, err := cl.run(ctx, newCmd("version"))
	if err != nil {
		return "", err
	}
	return parseVersion(out)
}

func parseVersion(out string) (string, error) {
	f := strings.Fields(out)
	if len(f) < 3 || f[0] != "dokku" || f[1] != "version" {
		return "", fmt.Errorf("unexpected dokku version output %q", strings.TrimSpace(out))
	}
	return strings.TrimPrefix(f[2], "v"), nil
}
