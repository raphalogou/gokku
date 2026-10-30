// Package snapshot holds Gokku's cached view of the Dokku server. The state is
// immutable and swapped atomically; readers never lock (DESIGN.md, 3.4).
package snapshot

import (
	"slices"
	"strings"
	"time"

	"gokku/internal/dokku"
)

// Status is the derived state of an app (APP-2).
type Status string

// App statuses, from worst to best for project health (PRJ-3).
const (
	StatusFailed    Status = "failed"
	StatusDeploying Status = "deploying"
	StatusRunning   Status = "running"
	StatusStopped   Status = "stopped"
)

// State is one consistent view of the server. Never modify a State after it is published.
type State struct {
	Server   Server
	Apps     []App     // sorted by name
	Services []Service // sorted by type, then name
}

// Server describes the Dokku host and Gokku's connection to it (SRV-1).
type Server struct {
	Reachable    bool
	LastOKAt     time.Time // last successful refresh; zero if none yet
	Error        string    // why the last refresh failed, empty when reachable
	ErrorCommand string    // the command that failed, for diagnostics
	Probed       bool      // capabilities hold a successful probe
	Capabilities dokku.Capabilities
}

// App is an app with its derived status.
type App struct {
	Name         string
	Status       Status
	Deployed     bool
	Locked       bool
	CreatedAt    time.Time
	DeploySource string
	Revision     string // git sha of the deployed code, or empty
	SourceImage  string
	DeployedAt   time.Time // when the deploy branch last moved; zero if never
	Builder      string
	Domains      []string
	Processes    []Process
	Services     []ServiceRef // services linked to the app
}

// Process is one process type and its containers.
type Process struct {
	Type    string
	Count   int
	Running int
}

// Service is a datastore service.
type Service struct {
	Type    string
	Name    string
	Status  string // docker state: running, exited, paused, missing, ...
	Version string
	Links   []string // app names
}

// ServiceRef names a service.
type ServiceRef struct {
	Type string
	Name string
}

// Running reports whether the service container is running.
func (s Service) Running() bool { return s.Status == "running" }

// App returns the app with name, if it exists.
func (s *State) App(name string) (App, bool) {
	i, ok := slices.BinarySearchFunc(s.Apps, name, func(a App, n string) int { return strings.Compare(a.Name, n) })
	if !ok {
		return App{}, false
	}
	return s.Apps[i], true
}

// Service returns the service, if it exists.
func (s *State) Service(typ, name string) (Service, bool) {
	for _, svc := range s.Services {
		if svc.Type == typ && svc.Name == name {
			return svc, true
		}
	}
	return Service{}, false
}

// JobActivity is what Gokku's own jobs say about an app, for the status derivation.
type JobActivity struct {
	Running    bool // a deploy, rebuild or restart job is running
	LastFailed bool // the latest finished deploy, rebuild or restart job failed
}

// deriveStatus implements DESIGN.md "Status derivation" (APP-2). An app locked by a
// deploy that Gokku did not start, such as a git push, also counts as deploying.
func deriveStatus(r *dokku.AppReport, job JobActivity) Status {
	switch {
	case job.Running || r.Locked:
		return StatusDeploying
	case job.LastFailed:
		return StatusFailed
	case r.Deployed && r.Running == "true":
		return StatusRunning
	default:
		return StatusStopped
	}
}

// build turns the reports into a State's apps and services.
func build(reports map[string]*dokku.AppReport, services []dokku.Service, jobs map[string]JobActivity) ([]App, []Service) {
	links := map[string][]ServiceRef{}
	outServices := make([]Service, 0, len(services))
	for _, s := range services {
		outServices = append(outServices, Service{
			Type: s.Type, Name: s.Name, Status: s.Status, Version: s.Version, Links: s.Links,
		})
		for _, app := range s.Links {
			links[app] = append(links[app], ServiceRef{Type: s.Type, Name: s.Name})
		}
	}
	slices.SortFunc(outServices, func(a, b Service) int {
		if c := strings.Compare(a.Type, b.Type); c != 0 {
			return c
		}
		return strings.Compare(a.Name, b.Name)
	})

	apps := make([]App, 0, len(reports))
	for _, r := range reports {
		refs := links[r.Name]
		slices.SortFunc(refs, func(a, b ServiceRef) int { return strings.Compare(a.Name, b.Name) })
		apps = append(apps, App{
			Name:         r.Name,
			Status:       deriveStatus(r, jobs[r.Name]),
			Deployed:     r.Deployed,
			Locked:       r.Locked,
			CreatedAt:    r.CreatedAt,
			DeploySource: r.DeploySource,
			Revision:     r.GitSHA,
			SourceImage:  r.SourceImage,
			DeployedAt:   r.GitUpdatedAt,
			Builder:      r.Builder,
			Domains:      r.Domains,
			Processes:    processes(r.Containers),
			Services:     refs,
		})
	}
	slices.SortFunc(apps, func(a, b App) int { return strings.Compare(a.Name, b.Name) })
	return apps, outServices
}

func processes(cs []dokku.Container) []Process {
	var out []Process
	for _, c := range cs {
		if len(out) == 0 || out[len(out)-1].Type != c.Type {
			out = append(out, Process{Type: c.Type})
		}
		p := &out[len(out)-1]
		p.Count++
		if c.State == "running" {
			p.Running++
		}
	}
	return out
}

// Health is the worst status among apps: failed, then deploying, then running (PRJ-3).
// A project whose apps are all stopped, or that has none, is stopped.
func Health(statuses []Status) Status {
	rank := map[Status]int{StatusFailed: 3, StatusDeploying: 2, StatusRunning: 1}
	worst := StatusStopped
	for _, s := range statuses {
		if rank[s] > rank[worst] {
			worst = s
		}
	}
	return worst
}
