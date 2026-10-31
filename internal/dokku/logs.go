package dokku

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// LogLine is one line of app output.
type LogLine struct {
	Time    time.Time // zero when the line carries no timestamp
	Process string    // dyno, such as web.1; empty when unknown
	Text    string
}

// Log tail sizes the UI offers (LOG-1).
var LogTailSizes = []int{100, 500, 1000}

var (
	ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)
	// Dokku's docker-local scheduler prefixes each docker log line:
	//   \x1b[36m2026-10-30T09:14:02.123456789Z app[web.1]:\x1b[0m GET /posts
	// (plugins/scheduler-docker-local/scheduler-logs).
	logLineRe = regexp.MustCompile(`^(\S+Z) app\[([^\]]+)\]: ?(.*)$`)
	// A process filter is a process type (web) or one container of it (web.1).
	processRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_-]{0,62}(\.[0-9]{1,4})?$`)
)

// ValidProcess reports whether p is a usable process filter.
func ValidProcess(p string) bool { return processRe.MatchString(p) }

func parseLogLine(raw string) LogLine {
	raw = strings.TrimRight(ansiRe.ReplaceAllString(raw, ""), "\r")
	m := logLineRe.FindStringSubmatch(raw)
	if m == nil {
		return LogLine{Text: raw}
	}
	t, err := time.Parse(time.RFC3339Nano, m[1])
	if err != nil {
		return LogLine{Text: raw}
	}
	return LogLine{Time: t.UTC(), Process: m[2], Text: m[3]}
}

// logArgs builds `logs <app> [--num N] [--ps type] [--tail]`. Dokku filters by process
// type; a container filter such as web.1 is applied to the parsed lines.
func logArgs(app, process string, num int, follow bool) (cmd, string, error) {
	if !ValidAppName(app) {
		return cmd{}, "", fmt.Errorf("%w: app %q", ErrInvalidName, app)
	}
	if process != "" && !ValidProcess(process) {
		return cmd{}, "", fmt.Errorf("%w: process %q", ErrInvalidName, process)
	}
	c := newCmd("logs", app, "--num", strconv.Itoa(num))
	typ, dyno := process, ""
	if i := strings.IndexByte(process, '.'); i >= 0 {
		typ, dyno = process[:i], process
	}
	if typ != "" {
		c = newCmd(append(c.args, "--ps", typ)...)
	}
	if follow {
		c = newCmd(append(c.args, "--tail")...)
	}
	return c, dyno, nil
}

// Logs returns the last num lines of the app's output, oldest first (LOG-1).
func (cl *Client) Logs(ctx context.Context, app, process string, num int) ([]LogLine, error) {
	c, dyno, err := logArgs(app, process, num, false)
	if err != nil {
		return nil, err
	}
	out, err := cl.run(ctx, c)
	if err != nil {
		return nil, err
	}
	return parseLogs(out, dyno, num), nil
}

// parseLogs parses the output of `dokku logs`. Dokku prints each container's lines in
// turn, so they are merged by time; dyno keeps one container; only the last num remain.
func parseLogs(out, dyno string, num int) []LogLine {
	var lines []LogLine
	for raw := range strings.Lines(out) {
		raw = strings.TrimRight(raw, "\n")
		if strings.TrimSpace(ansiRe.ReplaceAllString(raw, "")) == "" {
			continue
		}
		l := parseLogLine(raw)
		if dyno != "" && l.Process != dyno {
			continue
		}
		lines = append(lines, l)
	}
	slices.SortStableFunc(lines, func(a, b LogLine) int { return a.Time.Compare(b.Time) })
	if num > 0 && len(lines) > num {
		lines = lines[len(lines)-num:]
	}
	return lines
}

// FollowLogs streams new output lines until ctx ends (LOG-2). It runs one
// `dokku logs --tail` process, which ends when ctx is canceled (LOG-4).
func (cl *Client) FollowLogs(ctx context.Context, app, process string, onLine func(LogLine)) error {
	c, dyno, err := logArgs(app, process, 0, true)
	if err != nil {
		return err
	}
	return cl.stream(ctx, c, nil, func(l Line) {
		ll := parseLogLine(l.Text)
		if ll.Text == "" && ll.Process == "" {
			return
		}
		if dyno == "" || ll.Process == dyno {
			onLine(ll)
		}
	})
}
