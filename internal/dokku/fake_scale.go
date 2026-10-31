package dokku

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// scaleCommand answers ps:scale reads and writes. The caller holds f.mu.
func (f *FakeRunner) scaleCommand(args []string) (script, bool) {
	if args[0] != "ps:scale" {
		return script{}, false
	}
	if len(args) == 4 && args[1] == "--format" && args[2] == "json" {
		a, ok := f.apps[args[3]]
		if !ok {
			return failScript(fmt.Sprintf("App %s does not exist", args[3])), true
		}
		type row struct {
			Type     string `json:"process_type"`
			Quantity int    `json:"quantity"`
		}
		rows := []row{}
		for _, typ := range slices.Sorted(maps.Keys(a.processes)) {
			rows = append(rows, row{typ, a.processes[typ]})
		}
		b, _ := json.Marshal(rows)
		return script{lines: out(string(b))}, true
	}
	if len(args) < 3 {
		return script{}, false
	}
	a, ok := f.apps[args[1]]
	if !ok {
		return failScript(fmt.Sprintf("App %s does not exist", args[1])), true
	}
	lines := []string{"-----> Scaling " + a.name + " processes: " + strings.Join(args[2:], " ")}
	for _, kv := range args[2:] {
		typ, n, _ := strings.Cut(kv, "=")
		count, err := strconv.Atoi(n)
		if err != nil {
			return failScript("Invalid scale " + kv), true
		}
		if count == 0 {
			delete(a.processes, typ)
		} else {
			a.processes[typ] = count
		}
		lines = append(lines, fmt.Sprintf("-----> Deploying %s (count=%d)", typ, count))
	}
	return script{lines: out(append(lines, "=====> Application deployed")...)}, true
}
