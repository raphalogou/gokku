package dokku

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

// serviceCommand answers <type>:create, :destroy, :link, :unlink, :start, :stop and
// :restart, and the global domain commands. The caller holds f.mu.
func (f *FakeRunner) serviceCommand(args []string) (script, bool) {
	if sc, ok := f.globalDomainCommand(args); ok {
		return sc, true
	}
	typ, action, ok := strings.Cut(args[0], ":")
	if !ok || !IsDatastore(typ) || len(args) < 2 {
		return script{}, false
	}
	if !f.installed(typ) {
		return failScript(fmt.Sprintf("`%s` is not a dokku command.", args[0])), true
	}
	name := args[1]
	key := typ + "/" + name
	s, exists := f.services[key]
	title := strings.ToUpper(typ[:1]) + typ[1:]
	switch {
	case action == "create" && len(args) == 2:
		if exists {
			return failScript(fmt.Sprintf("%s service %s already exists", title, name)), true
		}
		version := map[string]string{"postgres": "postgres:17.6", "redis": "redis:8.2.1"}[typ]
		s = &fakeService{typ: typ, name: name, status: "running", version: cmp.Or(version, typ+":latest")}
		f.services[key] = s
		return script{lines: append(out(
			"       Waiting for "+name+" container to be ready (timeout: 60s)",
			"=====> "+title+" container created: "+name,
			"=====> "+name+" "+typ+" information",
		), out("       Dsn:                           "+fakeDSN(s))...)}, true
	case !exists:
		return failScript(fmt.Sprintf("%s service %s does not exist", title, name)), true
	case action == "destroy" && len(args) == 3 && args[2] == "--force":
		if len(s.links) > 0 {
			return failScript("Cannot delete linked service"), true
		}
		delete(f.services, key)
		return script{lines: out("=====> Deleting "+name, "=====> "+title+" container deleted: "+name)}, true
	case (action == "link" || action == "unlink") && len(args) == 3:
		a, ok := f.apps[args[2]]
		if !ok {
			return failScript(fmt.Sprintf("App %s does not exist", args[2])), true
		}
		env := LinkEnvVar(typ)
		linked := slices.Contains(s.links, a.name)
		if action == "link" {
			if linked {
				return failScript("Already linked as " + env), true
			}
			s.links = append(s.links, a.name)
			slices.Sort(s.links)
			a.config[env] = fakeDSN(s)
			return script{lines: out("-----> Setting config vars", "       "+env+":  "+fakeDSN(s),
				"-----> Restarting app "+a.name, "=====> Application deployed:")}, true
		}
		if !linked {
			return failScript("Not linked to app " + a.name), true
		}
		s.links = slices.DeleteFunc(s.links, func(l string) bool { return l == a.name })
		delete(a.config, env)
		return script{lines: out("-----> Unsetting "+env, "-----> Restarting app "+a.name, "=====> Application deployed:")}, true
	case len(args) == 2 && (action == "start" || action == "stop" || action == "restart"):
		if action == "stop" {
			s.status = "exited"
			return script{lines: out("=====> Stopping container", "       Container stopped")}, true
		}
		s.status = "running"
		return script{lines: out("=====> Starting container", "       Container started: "+name)}, true
	}
	return script{}, false
}

func (f *FakeRunner) globalDomainCommand(args []string) (script, bool) {
	switch {
	case slices.Equal(args, []string{"domains:report", "--global", "--format", "json"}):
		enabled := "false"
		if len(f.globalDomains) > 0 {
			enabled = "true"
		}
		return script{lines: out(fmt.Sprintf(`{"global-enabled":%q,"global-vhosts":%q}`, enabled,
			strings.Join(f.globalDomains, " ")))}, true
	case len(args) == 2 && args[0] == "domains:add-global":
		if !slices.Contains(f.globalDomains, args[1]) {
			f.globalDomains = append(f.globalDomains, args[1])
		}
		return script{lines: out("-----> Added " + args[1])}, true
	case len(args) == 2 && args[0] == "domains:remove-global":
		f.globalDomains = slices.DeleteFunc(f.globalDomains, func(d string) bool { return d == args[1] })
		return script{lines: out("-----> Removed " + args[1])}, true
	}
	return script{}, false
}
