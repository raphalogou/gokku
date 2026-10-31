package dokku

import (
	"fmt"
	"maps"
	"time"
)

// manageCommand answers apps:create, apps:rename and apps:destroy. The caller holds f.mu.
func (f *FakeRunner) manageCommand(args []string) (script, bool) {
	switch {
	case len(args) == 2 && args[0] == "apps:create":
		if _, ok := f.apps[args[1]]; ok {
			return failScript("Name is already taken"), true
		}
		f.apps[args[1]] = &fakeApp{name: args[1], createdAt: time.Now().Truncate(time.Second), restore: true,
			processes: map[string]int{}, config: map[string]string{}}
		return script{lines: out("-----> Creating " + args[1] + "...")}, true
	case len(args) == 3 && args[0] == "apps:rename":
		a, ok := f.apps[args[1]]
		if !ok {
			return failScript(fmt.Sprintf("App %s does not exist", args[1])), true
		}
		if _, taken := f.apps[args[2]]; taken {
			return failScript("Name is already taken"), true
		}
		delete(f.apps, args[1])
		a.name = args[2]
		f.apps[a.name] = a
		for _, s := range f.services {
			for i, l := range s.links {
				if l == args[1] {
					s.links[i] = a.name
				}
			}
		}
		lines := []string{"-----> Renaming " + args[1] + " to " + args[2], "Destroying " + args[1] + " (including all add-ons)",
			"-----> Cleaning up...", "-----> Building " + a.name + " from " + a.builder + "..."}
		for typ, n := range maps.All(a.processes) {
			lines = append(lines, fmt.Sprintf("-----> Deploying %s (count=%d)", typ, n))
		}
		return script{lines: out(append(lines, "=====> Application deployed")...)}, true
	case len(args) == 3 && args[0] == "apps:destroy" && args[1] == "--force":
		if _, ok := f.apps[args[2]]; !ok {
			return failScript(fmt.Sprintf("App %s does not exist", args[2])), true
		}
		delete(f.apps, args[2])
		for _, s := range f.services {
			for i := len(s.links) - 1; i >= 0; i-- {
				if s.links[i] == args[2] {
					s.links = append(s.links[:i], s.links[i+1:]...)
				}
			}
		}
		return script{lines: out("-----> Destroying "+args[2]+" (including all add-ons)", "-----> Cleaning up...")}, true
	}
	return script{}, false
}
