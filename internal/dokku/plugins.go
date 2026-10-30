package dokku

import (
	"bufio"
	"context"
	"fmt"
	"strings"
)

// Plugin is one line of plugin:list.
type Plugin struct {
	Name        string
	Version     string
	Enabled     bool
	Description string
}

// Plugins returns the installed plugins from `dokku plugin:list`.
func (cl *Client) Plugins(ctx context.Context) ([]Plugin, error) {
	out, err := cl.run(ctx, newCmd("plugin:list"))
	if err != nil {
		return nil, err
	}
	return parsePluginList(out)
}

// parsePluginList reads the text format, which every Dokku version prints:
//
//	plugn: 0.17.0
//	  00_dokku-standard    0.38.34 enabled    dokku core standard plugin
func parsePluginList(out string) ([]Plugin, error) {
	var plugins []Plugin
	sc := bufio.NewScanner(strings.NewReader(out))
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		if strings.HasPrefix(line, "plugn:") || strings.TrimSpace(line) == "" {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 3 || (f[2] != "enabled" && f[2] != "disabled") || !pluginNameRe.MatchString(f[0]) {
			return nil, fmt.Errorf("plugin:list line %d: unexpected %q", n, line)
		}
		plugins = append(plugins, Plugin{
			Name:        f[0],
			Version:     f[1],
			Enabled:     f[2] == "enabled",
			Description: strings.Join(f[3:], " "),
		})
	}
	return plugins, sc.Err()
}
