package dokku

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
)

// fakeDSN is a fake service's connection string.
func fakeDSN(s *fakeService) string {
	return fmt.Sprintf("%s://%s:fake-password@dokku-%s-%s:5432/%s", s.typ, s.typ, s.typ, s.name,
		strings.ReplaceAll(s.name, "-", "_"))
}

// seedConfig gives each app the variables its service links set, plus a few of its own.
func (f *FakeRunner) seedConfig() {
	for _, a := range f.apps {
		a.config = map[string]string{"GIT_REV": a.sha}
	}
	for _, s := range f.services {
		for _, app := range s.links {
			if a := f.apps[app]; a != nil {
				a.config[LinkEnvVar(s.typ)] = fakeDSN(s)
			}
		}
	}
	if a := f.apps["blog-api"]; a != nil {
		a.config["NODE_ENV"] = "production"
		a.config["SESSION_SECRET"] = "fake-session-secret-0123456789"
		a.config["SENTRY_DSN"] = "https://fake@sentry.example.com/42"
		a.config["TZ"] = "Europe/Paris"
	}
}

// configCommand answers config:export, config:import, config:unset and <type>:info --dsn.
// The caller holds f.mu.
func (f *FakeRunner) configCommand(args []string, stdin io.Reader) (script, bool) {
	switch {
	case len(args) == 4 && args[0] == "config:export" && args[1] == "--format" && args[2] == "json":
		a, ok := f.apps[args[3]]
		if !ok {
			return failScript(fmt.Sprintf("App %s does not exist", args[3])), true
		}
		b, _ := json.Marshal(a.config)
		return script{lines: out(string(b))}, true
	case len(args) == 6 && args[0] == "config:import" && args[5] == "-":
		a, ok := f.apps[args[4]]
		if !ok {
			return failScript(fmt.Sprintf("App %s does not exist", args[4])), true
		}
		vars := map[string]string{}
		if stdin == nil || json.NewDecoder(stdin).Decode(&vars) != nil || len(vars) == 0 {
			return failScript("No config vars to set"), true
		}
		lines := []string{"-----> Reading config vars from stdin"}
		for _, k := range slices.Sorted(maps.Keys(vars)) {
			a.config[k] = vars[k]
			// Dokku prints the values it sets; callers must not log this.
			lines = append(lines, fmt.Sprintf("-----> Setting config var %s=%s", k, vars[k]))
		}
		return script{lines: out(lines...)}, true
	case len(args) >= 4 && args[0] == "config:unset" && args[1] == "--no-restart":
		a, ok := f.apps[args[2]]
		if !ok {
			return failScript(fmt.Sprintf("App %s does not exist", args[2])), true
		}
		for _, k := range args[3:] {
			delete(a.config, k)
		}
		return script{lines: out("-----> Unsetting " + strings.Join(args[3:], ", "))}, true
	case len(args) == 3 && args[2] == "--dsn" && strings.HasSuffix(args[0], ":info"):
		s, ok := f.services[strings.TrimSuffix(args[0], ":info")+"/"+args[1]]
		if !ok {
			return failScript(fmt.Sprintf("service %s does not exist", args[1])), true
		}
		return script{lines: out(fakeDSN(s))}, true
	}
	return script{}, false
}
