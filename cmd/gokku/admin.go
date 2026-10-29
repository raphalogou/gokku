package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"gokku/internal/audit"
	"gokku/internal/auth"
	"gokku/internal/config"
	"gokku/internal/store"
)

// resetPassword implements `gokku admin reset-password <username>` (AUTH-8). On a terminal
// it prompts twice without echo; otherwise it reads one line from stdin, for scripts.
func resetPassword(args []string, stdin io.Reader, stdout io.Writer) error {
	cfg, rest, err := config.FromOSArgs(args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return errors.New("usage: gokku admin reset-password [flags] <username>")
	}
	username := rest[0]

	password, err := readNewPassword(stdin, stdout)
	if err != nil {
		return err
	}

	ctx := context.Background()
	log := newLogger(cfg)
	db, _, err := openState(ctx, cfg, log)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	svc, err := auth.New(db, auth.Options{})
	if err != nil {
		return err
	}
	u, err := svc.ResetPassword(ctx, username, password)
	if errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("no user named %q", username)
	}
	var ve *auth.ValidationError
	if errors.As(err, &ve) {
		return errors.New(ve.Fields["password"])
	}
	if err != nil {
		return err
	}
	audit.New(db, log).RecordSystem(ctx, audit.Entry{Action: "users:reset-password", TargetType: "user", TargetName: u.Username})
	_, err = fmt.Fprintf(stdout, "Password for %s changed. All of their sessions are signed out.\n", u.Username)
	return err
}

func readNewPassword(stdin io.Reader, stdout io.Writer) (string, error) {
	f, ok := stdin.(*os.File)
	if !ok || !term.IsTerminal(int(f.Fd())) {
		line, err := bufio.NewReader(stdin).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			return "", errors.New("no password on stdin")
		}
		return line, nil
	}

	fd := int(f.Fd())
	prompt := func(label string) (string, error) {
		if _, err := fmt.Fprint(stdout, label); err != nil {
			return "", err
		}
		b, err := term.ReadPassword(fd)
		_, _ = fmt.Fprintln(stdout)
		return string(b), err
	}
	first, err := prompt("New password: ")
	if err != nil {
		return "", err
	}
	second, err := prompt("Repeat password: ")
	if err != nil {
		return "", err
	}
	if first != second {
		return "", errors.New("the passwords do not match")
	}
	return first, nil
}
