package dokku

import (
	"slices"
	"strings"
)

// redacted replaces secret argument values in logs, audit details and errors (CMD-3).
const redacted = "[redacted]"

// cmd is a dokku argument list that knows which positions hold secrets.
type cmd struct {
	args   []string
	secret []bool
	hidden []string // secret values passed outside the arguments
}

func newCmd(args ...string) cmd {
	return cmd{args: args, secret: make([]bool, len(args))}
}

// secretArg appends an argument whose value must never be logged.
func (c cmd) secretArg(v string) cmd {
	c.args = append(c.args, v)
	c.secret = append(c.secret, true)
	return c
}

// secretValue records a value that is not an argument, for example one sent on stdin, so
// it is scrubbed from the command's output and errors.
func (c cmd) secretValue(v string) cmd {
	c.hidden = append(slices.Clone(c.hidden), v)
	return c
}

// String renders the command for logs and messages with secrets redacted.
func (c cmd) String() string {
	parts := make([]string, 0, len(c.args)+1)
	parts = append(parts, "dokku")
	for i, a := range c.args {
		if c.secret[i] {
			a = redacted
		}
		parts = append(parts, a)
	}
	return strings.Join(parts, " ")
}

// secrets returns the secret values, for scrubbing them from command output.
func (c cmd) secrets() []string {
	var out []string
	for i, a := range c.args {
		if c.secret[i] && a != "" {
			out = append(out, a)
		}
	}
	for _, v := range c.hidden {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

// minScrubLength is the shortest secret value scrubbed from output; replacing shorter ones
// such as "1" would mangle unrelated text.
const minScrubLength = 4

// scrub removes every secret value of c from s.
func (c cmd) scrub(s string) string {
	for _, v := range c.secrets() {
		if len(v) >= minScrubLength {
			s = strings.ReplaceAll(s, v, redacted)
		}
	}
	return s
}
