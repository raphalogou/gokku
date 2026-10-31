package dokku

import (
	"context"
	"fmt"
	"net/mail"
	"time"
)

// LetsencryptReport is an app's Let's Encrypt state from `letsencrypt:report <app>`.
type LetsencryptReport struct {
	Active        bool
	AutoRenew     bool      // the plugin's renewal cron job is installed (server-wide)
	Expires       time.Time // zero without a certificate
	Email         string    // set for the app
	ComputedEmail string    // the app's, else the global one
}

// Letsencrypt reads the app's report. The command needs the letsencrypt plugin (SSL-1).
func (cl *Client) Letsencrypt(ctx context.Context, app string) (LetsencryptReport, error) {
	if !ValidAppName(app) {
		return LetsencryptReport{}, fmt.Errorf("%w: app %q", ErrInvalidName, app)
	}
	out, err := cl.run(ctx, newCmd("letsencrypt:report", app))
	if err != nil {
		return LetsencryptReport{}, err
	}
	return parseLetsencrypt(out, app)
}

func parseLetsencrypt(out, app string) (LetsencryptReport, error) {
	sections, err := parseReport(out, "letsencrypt")
	if err != nil {
		return LetsencryptReport{}, err
	}
	f, ok := sections[app]
	if !ok {
		return LetsencryptReport{}, fmt.Errorf("letsencrypt:report: no section for %s", app)
	}
	return LetsencryptReport{
		Active:        f.bool("letsencrypt-active"),
		AutoRenew:     f.bool("letsencrypt-autorenew"),
		Expires:       unixTime(f["letsencrypt-expiration"]),
		Email:         f["letsencrypt-email"],
		ComputedEmail: f["letsencrypt-computed-email"],
	}, nil
}

// EnableLetsencrypt requests a certificate, or renews one with force (SSL-1), streaming
// the output.
func (cl *Client) EnableLetsencrypt(ctx context.Context, app string, force bool, onLine func(Line)) error {
	if !ValidAppName(app) {
		return fmt.Errorf("%w: app %q", ErrInvalidName, app)
	}
	c := newCmd("letsencrypt:enable", app)
	if force {
		c = newCmd("letsencrypt:enable", app, "--force")
	}
	return cl.stream(ctx, c, nil, onLine)
}

// DisableLetsencrypt removes the app's certificate, streaming the output.
func (cl *Client) DisableLetsencrypt(ctx context.Context, app string, onLine func(Line)) error {
	if !ValidAppName(app) {
		return fmt.Errorf("%w: app %q", ErrInvalidName, app)
	}
	return cl.stream(ctx, newCmd("letsencrypt:disable", app), nil, onLine)
}

// SetLetsencryptEmail sets the app's contact email, or clears it when empty.
func (cl *Client) SetLetsencryptEmail(ctx context.Context, app, email string) error {
	if !ValidAppName(app) {
		return fmt.Errorf("%w: app %q", ErrInvalidName, app)
	}
	c := newCmd("letsencrypt:set", app, "email")
	if email != "" {
		if a, err := mail.ParseAddress(email); err != nil || a.Address != email {
			return fmt.Errorf("%w: email %q", ErrInvalidName, email)
		}
		c = newCmd("letsencrypt:set", app, "email", email)
	}
	_, err := cl.run(ctx, c)
	return err
}

// SetLetsencryptAutoRenew installs or removes the plugin's renewal cron job.
func (cl *Client) SetLetsencryptAutoRenew(ctx context.Context, on bool) error {
	flag := "--remove"
	if on {
		flag = "--add"
	}
	_, err := cl.run(ctx, newCmd("letsencrypt:cron-job", flag))
	return err
}
