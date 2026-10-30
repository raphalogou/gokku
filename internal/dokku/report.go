package dokku

import (
	"bufio"
	"fmt"
	"strings"
)

// Fields are the key: value lines of one report section. Keys are normalized: lower
// case with dashes, so "Ps can scale" becomes "ps-can-scale".
type Fields map[string]string

// parseReport parses the text format shared by core `<plugin>:report` and datastore
// `<type>:info` when they run without a target, which prints one section per app or
// service:
//
//	=====> node-js-app ps information
//	       Deployed:                      false
//	       Running:                       false
//
// The text format is used rather than --format json because the JSON variant prints
// one object per target without naming it (docs/DECISIONS.md).
func parseReport(out, kind string) (map[string]Fields, error) {
	sections := map[string]Fields{}
	var cur Fields
	suffix := " " + kind + " information"
	sc := bufio.NewScanner(strings.NewReader(out))
	sc.Buffer(make([]byte, 0, 64<<10), maxOutputBytes)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimRight(sc.Text(), " \r")
		switch {
		case strings.HasPrefix(line, "=====> "):
			head := strings.TrimPrefix(line, "=====> ")
			name, ok := strings.CutSuffix(head, suffix)
			if !ok || name == "" || strings.ContainsAny(name, " \t") {
				return nil, fmt.Errorf("%s report line %d: unexpected header %q", kind, n, line)
			}
			cur = Fields{}
			sections[name] = cur
		case strings.TrimSpace(line) == "":
		case strings.HasPrefix(line, " !"):
			// a warning, such as "There are no apps"
		default:
			if cur == nil {
				return nil, fmt.Errorf("%s report line %d: field before any header", kind, n)
			}
			key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
			if !ok {
				return nil, fmt.Errorf("%s report line %d: expected \"key: value\"", kind, n)
			}
			cur[normalizeKey(key)] = strings.TrimSpace(value)
		}
	}
	return sections, sc.Err()
}

func normalizeKey(k string) string {
	return strings.Join(strings.Fields(strings.ToLower(k)), "-")
}

// bool reports whether the field is "true".
func (f Fields) bool(key string) bool { return f[key] == "true" }

// list splits a field on sep, dropping empty items.
func (f Fields) list(key, sep string) []string {
	var out []string
	for item := range strings.SplitSeq(f[key], sep) {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}
