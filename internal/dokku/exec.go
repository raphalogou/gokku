package dokku

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
)

// killDelay is how long a canceled command gets between SIGTERM and SIGKILL (JOB-4).
const killDelay = 10 * time.Second

// ExecRunner runs `sudo -n <bin> args...` (or `<bin> args...` without sudo) as a child
// process in its own process group, so that cancel reaches the whole tree.
type ExecRunner struct {
	Bin  string // path of the dokku binary
	Sudo bool   // run through sudo -n (SEC-7)
}

func (r *ExecRunner) command(ctx context.Context, args []string) *exec.Cmd {
	name, argv := r.Bin, args
	if r.Sudo {
		name, argv = "sudo", append([]string{"-n", r.Bin}, args...)
	}
	c := exec.CommandContext(ctx, name, argv...) //nolint:gosec // argument array, no shell; identifiers are validated by the typed wrappers
	c.Env = []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C.UTF-8"}
	setProcessGroup(c)
	c.Cancel = func() error {
		terminateGroup(c, false)
		time.AfterFunc(killDelay, func() { terminateGroup(c, true) })
		return nil
	}
	c.WaitDelay = killDelay + time.Second
	return c
}

// Run implements Runner.
func (r *ExecRunner) Run(ctx context.Context, args ...string) (Result, error) {
	c := r.command(ctx, args)
	var stdout, stderr limitedBuffer
	stdout.max, stderr.max = maxOutputBytes, maxOutputBytes
	c.Stdout, c.Stderr = &stdout, &stderr
	start := time.Now()
	err := c.Run()
	res := Result{Stdout: stdout.String(), Stderr: stderr.String(), Duration: time.Since(start)}
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok && ctx.Err() == nil {
		res.ExitCode = exitErr.ExitCode()
		return res, nil
	}
	if err != nil {
		return res, fmt.Errorf("run dokku: %w", err)
	}
	return res, nil
}

// Stream implements Runner.
func (r *ExecRunner) Stream(ctx context.Context, args []string, stdin io.Reader, onLine func(Line)) error {
	c := r.command(ctx, args)
	c.Stdin = stdin
	stdout, err := c.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := c.StderrPipe()
	if err != nil {
		return err
	}
	if err := c.Start(); err != nil {
		return fmt.Errorf("start dokku: %w", err)
	}

	var mu sync.Mutex
	emit := func(l Line) {
		mu.Lock()
		defer mu.Unlock()
		onLine(l)
	}
	var wg sync.WaitGroup
	wg.Go(func() { readLines(stdout, "stdout", emit) })
	wg.Go(func() { readLines(stderr, "stderr", emit) })
	wg.Wait()

	err = c.Wait()
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok && ctx.Err() == nil {
		return &ExitError{Command: newCmd(args...).String(), ExitCode: exitErr.ExitCode()}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

// readLines splits r into lines, truncating each at MaxLineBytes.
func readLines(r io.Reader, stream string, emit func(Line)) {
	br := bufio.NewReaderSize(r, 64<<10)
	var line []byte
	for {
		chunk, isPrefix, err := br.ReadLine()
		if len(line) < MaxLineBytes {
			line = append(line, chunk[:min(len(chunk), MaxLineBytes-len(line))]...)
		}
		if err != nil {
			if len(line) > 0 {
				emit(Line{Stream: stream, Text: string(line)})
			}
			return
		}
		if !isPrefix {
			emit(Line{Stream: stream, Text: string(bytes.TrimRight(line, "\r"))})
			line = line[:0]
		}
	}
}

// limitedBuffer keeps the first max bytes written to it and discards the rest.
type limitedBuffer struct {
	bytes.Buffer
	max int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if room := b.max - b.Len(); room > 0 {
		b.Buffer.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}
