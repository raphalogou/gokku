package dokku

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Minimum versions Gokku is built against (docs/COMPATIBILITY.md).
const (
	MinDokkuVersion     = "0.38.0"
	MinDatastoreVersion = "2.6.0" // datastore plugins built on dokku-datastore report every service at once
)

// Capabilities is what the probe found on the server (CMD-2).
type Capabilities struct {
	DokkuVersion string
	Plugins      []Plugin
	// Datastores lists the supported datastore plugins that are installed, enabled and
	// recent enough. Only these are queried.
	Datastores []string
	// Letsencrypt reports whether the letsencrypt plugin is installed and enabled.
	Letsencrypt bool
	// Unavailable explains, per plugin or feature, why it is hidden.
	Unavailable map[string]string
}

// Probe records the Dokku version and the installed plugins, and decides which features
// Gokku can offer. It runs at start and on rescan.
func (cl *Client) Probe(ctx context.Context) (Capabilities, error) {
	version, err := cl.Version(ctx)
	if err != nil {
		return Capabilities{}, err
	}
	plugins, err := cl.Plugins(ctx)
	if err != nil {
		return Capabilities{}, err
	}
	return capabilities(version, plugins), nil
}

func capabilities(version string, plugins []Plugin) Capabilities {
	c := Capabilities{DokkuVersion: version, Plugins: plugins, Unavailable: map[string]string{}}
	if compareVersions(version, MinDokkuVersion) < 0 {
		c.Unavailable["dokku"] = fmt.Sprintf("Dokku %s is older than %s, the oldest version Gokku supports.", version, MinDokkuVersion)
	}
	for _, p := range plugins {
		switch {
		case !p.Enabled:
			c.Unavailable[p.Name] = "The plugin is disabled."
		case IsDatastore(p.Name) && compareVersions(p.Version, MinDatastoreVersion) < 0:
			c.Unavailable[p.Name] = fmt.Sprintf("Version %s is older than %s. Update it with: sudo dokku plugin:update %s",
				p.Version, MinDatastoreVersion, p.Name)
		case IsDatastore(p.Name):
			c.Datastores = append(c.Datastores, p.Name)
		case p.Name == "letsencrypt":
			c.Letsencrypt = true
		}
	}
	slices.Sort(c.Datastores)
	return c
}

// compareVersions compares dotted numeric versions such as 0.38.34. Missing or
// non-numeric parts count as zero, so "2.6.0-rc1" compares equal to "2.6.0".
func compareVersions(a, b string) int {
	pa, pb := versionParts(a), versionParts(b)
	for i := range max(len(pa), len(pb)) {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

func versionParts(v string) []int {
	v = strings.TrimPrefix(v, "v")
	var out []int
	for part := range strings.SplitSeq(v, ".") {
		digits := part
		if i := strings.IndexFunc(part, func(r rune) bool { return r < '0' || r > '9' }); i >= 0 {
			digits = part[:i]
		}
		n, _ := strconv.Atoi(digits)
		out = append(out, n)
	}
	return out
}
