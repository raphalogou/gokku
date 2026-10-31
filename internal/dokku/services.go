package dokku

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// CreateService runs `dokku <type>:create <name>` (SVC-2). The plugin ends with the
// service's info report, which holds the connection string, so the output stops at the
// "container created" line.
func (cl *Client) CreateService(ctx context.Context, typ, name string, onLine func(Line)) error {
	if err := validService(typ, name); err != nil {
		return err
	}
	done := false
	return cl.serviceStream(ctx, newCmd(typ+":create", name), func(l Line) {
		if done {
			return
		}
		if strings.Contains(l.Text, "container created:") {
			done = true
		}
		onLine(l)
	})
}

// DestroyService runs `dokku <type>:destroy <name> --force` (SVC-2). The plugin refuses
// while apps are linked; callers check the links first.
func (cl *Client) DestroyService(ctx context.Context, typ, name string, onLine func(Line)) error {
	if err := validService(typ, name); err != nil {
		return err
	}
	return cl.serviceStream(ctx, newCmd(typ+":destroy", name, "--force"), onLine)
}

// LinkService runs `dokku <type>:link <name> <app>`, which sets the connection variable on
// the app and restarts it (SVC-3).
func (cl *Client) LinkService(ctx context.Context, typ, name, app string, onLine func(Line)) error {
	return cl.linkCommand(ctx, "link", typ, name, app, onLine)
}

// UnlinkService runs `dokku <type>:unlink <name> <app>` (SVC-3).
func (cl *Client) UnlinkService(ctx context.Context, typ, name, app string, onLine func(Line)) error {
	return cl.linkCommand(ctx, "unlink", typ, name, app, onLine)
}

func (cl *Client) linkCommand(ctx context.Context, action, typ, name, app string, onLine func(Line)) error {
	if err := validService(typ, name); err != nil {
		return err
	}
	if !ValidAppName(app) {
		return fmt.Errorf("%w: app %q", ErrInvalidName, app)
	}
	return cl.serviceStream(ctx, newCmd(typ+":"+action, name, app), onLine)
}

// ServiceActions are the lifecycle commands of a service (SVC-2).
var ServiceActions = []string{"start", "stop", "restart"}

// ServiceLifecycle runs `dokku <type>:<start|stop|restart> <name>`.
func (cl *Client) ServiceLifecycle(ctx context.Context, action, typ, name string, onLine func(Line)) error {
	if !strings.Contains(" start stop restart ", " "+action+" ") {
		return fmt.Errorf("%w: action %q", ErrInvalidName, action)
	}
	if err := validService(typ, name); err != nil {
		return err
	}
	return cl.serviceStream(ctx, newCmd(typ+":"+action, name), onLine)
}

func validService(typ, name string) error {
	if !IsDatastore(typ) {
		return fmt.Errorf("%w: service type %q", ErrInvalidName, typ)
	}
	if !ValidServiceName(name) {
		return fmt.Errorf("%w: service %q", ErrInvalidName, name)
	}
	return nil
}

// serviceStream streams a datastore command with connection strings masked. Linking prints
// the app's new config variable, and the plugins print a service's DSN, both with the
// password; the plugin's secrets are not known in advance, so they are masked by shape.
func (cl *Client) serviceStream(ctx context.Context, c cmd, onLine func(Line)) error {
	err := cl.stream(ctx, c, nil, func(l Line) {
		if dsnLine.MatchString(l.Text) {
			return
		}
		l.Text = RedactURLs(l.Text)
		onLine(l)
	})
	return redactErr(err)
}

// dsnLine is a "Dsn:" or "Exposed dsn:" line of a datastore info report.
var dsnLine = regexp.MustCompile(`(?i)^\s*(exposed\s+)?dsn:`)

// urlCredentials is the password part of a URL: scheme://user:password@host.
var urlCredentials = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://[^\s:/@]*:)[^\s@]+@`)

// RedactURLs masks the password of every URL in s.
func RedactURLs(s string) string {
	return urlCredentials.ReplaceAllString(s, "${1}"+redacted+"@")
}

func redactErr(err error) error {
	if exitErr, ok := errors.AsType[*ExitError](err); ok {
		return &ExitError{Command: exitErr.Command, ExitCode: exitErr.ExitCode, Stderr: RedactURLs(exitErr.Stderr)}
	}
	return err
}

// GlobalDomains returns the domains every app gets a subdomain of, from
// `dokku domains:report --global --format json` (SET-1).
func (cl *Client) GlobalDomains(ctx context.Context) ([]string, error) {
	out, err := cl.run(ctx, newCmd("domains:report", "--global", "--format", "json"))
	if err != nil {
		return nil, err
	}
	return parseGlobalDomains(out)
}

func parseGlobalDomains(out string) ([]string, error) {
	var r struct {
		Vhosts string `json:"global-vhosts"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &r); err != nil {
		return nil, fmt.Errorf("domains:report --global: %w", err)
	}
	domains := strings.Fields(r.Vhosts)
	if domains == nil {
		domains = []string{}
	}
	return domains, nil
}

// AddGlobalDomain runs `dokku domains:add-global <domain>`.
func (cl *Client) AddGlobalDomain(ctx context.Context, domain string) error {
	return cl.globalDomainCommand(ctx, "domains:add-global", domain)
}

// RemoveGlobalDomain runs `dokku domains:remove-global <domain>`.
func (cl *Client) RemoveGlobalDomain(ctx context.Context, domain string) error {
	return cl.globalDomainCommand(ctx, "domains:remove-global", domain)
}

func (cl *Client) globalDomainCommand(ctx context.Context, command, domain string) error {
	if !ValidDomain(domain) {
		return fmt.Errorf("%w: domain %q", ErrInvalidName, domain)
	}
	_, err := cl.run(ctx, newCmd(command, strings.ToLower(domain)))
	return err
}
