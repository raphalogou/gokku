// Package dokku is the only package that runs the dokku command (CMD-1). It defines the
// Runner interface with an exec and a fake implementation, validates identifiers, builds
// argument arrays, redacts secret arguments and parses command output.
package dokku

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// Runner executes dokku commands. Implementations never use a shell (SEC-1).
type Runner interface {
	// Run executes `dokku args...` and returns its output. The error is non-nil only
	// when the command could not run to completion (not found, canceled); a non-zero
	// exit is reported in Result.ExitCode.
	Run(ctx context.Context, args ...string) (Result, error)
	// Stream runs a command and calls onLine for each output line until it ends or ctx
	// is canceled. stdin may be nil. A non-zero exit is returned as *ExitError.
	Stream(ctx context.Context, args []string, stdin io.Reader, onLine func(Line)) error
}

// Result is the outcome of Run.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Duration time.Duration
}

// Line is one line of streamed output.
type Line struct {
	Stream string // "stdout" or "stderr"
	Text   string
}

// MaxLineBytes is the longest line a stream delivers; longer lines are truncated (JOB-3).
const MaxLineBytes = 16 << 10

// maxOutputBytes caps what Run keeps of each output stream.
const maxOutputBytes = 16 << 20

// ExitError reports a command that exited with a non-zero status.
type ExitError struct {
	Command  string // redacted, for messages
	ExitCode int
	Stderr   string // redacted and shortened
}

func (e *ExitError) Error() string {
	msg := strings.TrimSpace(e.Stderr)
	if msg == "" {
		return fmt.Sprintf("%s: exit status %d", e.Command, e.ExitCode)
	}
	return fmt.Sprintf("%s: %s", e.Command, msg)
}

// ErrInvalidName is returned when an identifier fails validation before any command runs.
var ErrInvalidName = errors.New("invalid name")

// ErrNotSupported is returned by the fake runner for commands it does not implement.
var ErrNotSupported = errors.New("command not supported")
