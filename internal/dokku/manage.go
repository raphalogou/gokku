package dokku

import (
	"context"
	"fmt"
)

// CreateApp runs `dokku apps:create <app>` (APP-3).
func (cl *Client) CreateApp(ctx context.Context, app string) error {
	if !ValidAppName(app) {
		return fmt.Errorf("%w: app %q", ErrInvalidName, app)
	}
	_, err := cl.run(ctx, newCmd("apps:create", app))
	return err
}

// RenameApp runs `dokku apps:rename <old> <new>`, which redeploys the app under its new
// name (APP-4), streaming the output.
func (cl *Client) RenameApp(ctx context.Context, from, to string, onLine func(Line)) error {
	if !ValidAppName(from) || !ValidAppName(to) {
		return fmt.Errorf("%w: app %q or %q", ErrInvalidName, from, to)
	}
	return cl.stream(ctx, newCmd("apps:rename", from, to), nil, onLine)
}

// DestroyApp runs `dokku apps:destroy --force <app>` (APP-5). Dokku reads its flags before
// the app name; linked services are not destroyed.
func (cl *Client) DestroyApp(ctx context.Context, app string, onLine func(Line)) error {
	if !ValidAppName(app) {
		return fmt.Errorf("%w: app %q", ErrInvalidName, app)
	}
	return cl.stream(ctx, newCmd("apps:destroy", "--force", app), nil, onLine)
}
