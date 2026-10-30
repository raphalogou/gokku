// Command gokku is a web GUI for Dokku.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"gokku/internal/api"
	"gokku/internal/audit"
	"gokku/internal/auth"
	"gokku/internal/config"
	"gokku/internal/dokku"
	"gokku/internal/events"
	"gokku/internal/secrets"
	"gokku/internal/server"
	"gokku/internal/snapshot"
	"gokku/internal/store"
	"gokku/internal/web"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

const usage = `Usage: gokku <command> [flags]

Commands:
  serve                              run the web server
  migrate                            apply database migrations and exit
  admin reset-password <username>    set a new password and sign the user out everywhere
  version                            print the version

Flags and GOKKU_* environment variables are the same for every command.
Run "gokku serve -h" to list them.
`

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "gokku:", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.Join(printUsage(stderr), errors.New("missing command"))
	}
	switch args[0] {
	case "serve":
		return serve(args[1:])
	case "migrate":
		return migrate(args[1:])
	case "admin":
		if len(args) < 2 || args[1] != "reset-password" {
			return errors.Join(printUsage(stderr), errors.New(`unknown admin command, expected "reset-password"`))
		}
		return resetPassword(args[2:], stdin, stdout)
	case "version", "--version":
		_, err := fmt.Fprintln(stdout, version)
		return err
	case "help", "-h", "--help":
		return printUsage(stdout)
	default:
		return errors.Join(printUsage(stderr), fmt.Errorf("unknown command %q", args[0]))
	}
}

func printUsage(w io.Writer) error {
	_, err := fmt.Fprint(w, usage)
	return err
}

func newLogger(cfg config.Config) *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel}))
}

// openState opens the database, applying migrations, and the secret key. It refuses to
// start when the key file does not match the one the database was created with.
func openState(ctx context.Context, cfg config.Config, log *slog.Logger) (*store.DB, *secrets.Box, error) {
	db, err := store.Open(ctx, cfg.DataDir, log)
	if err != nil {
		return nil, nil, err
	}
	box, err := secrets.LoadOrCreate(cfg.DataDir)
	if err != nil {
		_ = db.Close()
		return nil, nil, err
	}
	stored, ok, err := db.Setting(ctx, store.SettingSecretKeyFingerprint)
	switch {
	case err != nil:
	case !ok:
		err = db.SetSetting(ctx, store.SettingSecretKeyFingerprint, box.Fingerprint())
	case stored != box.Fingerprint():
		err = fmt.Errorf("%s/secret.key does not match this database (key fingerprint %s, database expects %s): "+
			"restore the original secret.key from your backup", cfg.DataDir, box.Fingerprint(), stored)
	}
	if err != nil {
		_ = db.Close()
		return nil, nil, err
	}
	return db, box, nil
}

func serve(args []string) error {
	cfg, err := config.FromOS(args)
	if err != nil {
		return err
	}
	log := newLogger(cfg)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, _, err := openState(ctx, cfg, log)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	authSvc, err := auth.New(db, auth.Options{})
	if err != nil {
		return err
	}
	runner, serverName, err := newRunner(ctx, cfg, db)
	if err != nil {
		return err
	}
	hub := events.NewHub()
	snap := snapshot.New(snapshot.Options{
		Client: dokku.NewClient(runner, log), Activity: snapshot.StoreActivity(db), Hub: hub, Log: log,
	})
	apiHandler, err := api.New(api.Options{
		Auth: authSvc, DB: db, Audit: audit.New(db, log), Log: log, Snapshot: snap, Hub: hub,
		ServerName: serverName, DokkuHost: cfg.DokkuHost,
	})
	if err != nil {
		return err
	}
	static, err := web.Dist()
	if err != nil {
		return err
	}
	handler, err := server.New(server.Options{
		Logger:         log,
		Version:        version,
		Static:         static,
		API:            apiHandler,
		Listen:         cfg.Listen,
		TrustedProxies: cfg.TrustedProxies,
	})
	if err != nil {
		return err
	}

	ln, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		// No WriteTimeout: SSE streams stay open (EVT-3).
		BaseContext: func(net.Listener) context.Context { return ctx },
	}

	go pruneSessions(ctx, db, log)
	go snap.Run(ctx)

	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	log.Info("listening", slog.String("addr", ln.Addr().String()), slog.String("version", version))

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}

	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	return nil
}

// newRunner returns the Dokku runner and the server name shown in the UI. The fake runner
// serves the mockup's data and fails while <data dir>/fake-fail exists (SRV-2).
func newRunner(ctx context.Context, cfg config.Config, db *store.DB) (dokku.Runner, string, error) {
	if cfg.Runner == config.RunnerFake {
		fake := dokku.NewFakeRunner(time.Now())
		fake.FailFile = filepath.Join(cfg.DataDir, "fake-fail")
		return fake, "dokku-01", seedFake(ctx, db)
	}
	name, err := os.Hostname()
	if err != nil {
		name = "dokku"
	}
	return &dokku.ExecRunner{Bin: cfg.DokkuBin, Sudo: cfg.DokkuSudo}, name, nil
}

// pruneSessions deletes expired sessions every hour until ctx ends.
func pruneSessions(ctx context.Context, db *store.DB, log *slog.Logger) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		if n, err := db.DeleteExpiredSessions(ctx); err != nil && ctx.Err() == nil {
			log.Error("prune sessions", slog.Any("error", err))
		} else if n > 0 {
			log.Info("pruned expired sessions", slog.Int64("count", n))
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func migrate(args []string) error {
	cfg, err := config.FromOS(args)
	if err != nil {
		return err
	}
	db, _, err := openState(context.Background(), cfg, newLogger(cfg))
	if err != nil {
		return err
	}
	return db.Close()
}
