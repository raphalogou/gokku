package dokku

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// domainRe is a host name: lowercase labels of letters, digits and dashes, not starting or
// ending with a dash, optionally under one leading wildcard.
var domainRe = regexp.MustCompile(`^(\*\.)?([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)*[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// ValidDomain reports whether d is a usable domain (DOM-1).
func ValidDomain(d string) bool { return len(d) <= 253 && domainRe.MatchString(d) }

// AddDomain adds a domain to an app with `dokku domains:add <app> <domain>`.
func (cl *Client) AddDomain(ctx context.Context, app, domain string) error {
	return cl.domainCommand(ctx, "domains:add", app, domain)
}

// RemoveDomain removes a domain with `dokku domains:remove <app> <domain>`.
func (cl *Client) RemoveDomain(ctx context.Context, app, domain string) error {
	return cl.domainCommand(ctx, "domains:remove", app, domain)
}

func (cl *Client) domainCommand(ctx context.Context, command, app, domain string) error {
	if !ValidAppName(app) {
		return fmt.Errorf("%w: app %q", ErrInvalidName, app)
	}
	if !ValidDomain(domain) {
		return fmt.Errorf("%w: domain %q", ErrInvalidName, domain)
	}
	_, err := cl.run(ctx, newCmd(command, app, strings.ToLower(domain)))
	return err
}
