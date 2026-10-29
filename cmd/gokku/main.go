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
	"syscall"
	"time"

	"gokku/internal/config"
	"gokku/internal/server"
	"gokku/internal/web"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

const usage = `Usage: gokku <command> [flags]

Commands:
  serve     run the web server
  version   print the version

Run "gokku serve -h" for the server flags.
`

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "gokku:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.Join(printUsage(stderr), errors.New("missing command"))
	}
	switch args[0] {
	case "serve":
		return serve(args[1:])
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

func serve(args []string) error {
	cfg, err := config.FromOS(args)
	if err != nil {
		return err
	}
	log := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel}))

	static, err := web.Dist()
	if err != nil {
		return err
	}
	handler, err := server.New(server.Options{Logger: log, Version: version, Static: static})
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

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
