package dokku

import (
	"context"
	"log/slog"
	"strings"
)

// Client runs typed dokku commands through a Runner. Every method validates its
// identifiers before anything runs (CMD-1) and logs commands with secrets redacted (CMD-3).
type Client struct {
	runner Runner
	log    *slog.Logger
}

// NewClient returns a client that runs commands through r.
func NewClient(r Runner, log *slog.Logger) *Client {
	return &Client{runner: r, log: log}
}

// maxErrorOutput bounds the stderr kept in an ExitError.
const maxErrorOutput = 2 << 10

// run executes c and returns its stdout. A non-zero exit becomes *ExitError.
func (cl *Client) run(ctx context.Context, c cmd) (string, error) {
	res, err := cl.runner.Run(ctx, c.args...)
	cl.log.LogAttrs(ctx, slog.LevelDebug, "dokku",
		slog.String("command", c.String()), slog.Int("exit", res.ExitCode), slog.Duration("duration", res.Duration))
	if err != nil {
		return "", err
	}
	if res.ExitCode != 0 {
		msg := strings.TrimSpace(res.Stderr)
		if msg == "" {
			msg = strings.TrimSpace(res.Stdout)
		}
		if len(msg) > maxErrorOutput {
			msg = msg[:maxErrorOutput]
		}
		return "", &ExitError{Command: c.String(), ExitCode: res.ExitCode, Stderr: c.scrub(msg)}
	}
	return res.Stdout, nil
}
