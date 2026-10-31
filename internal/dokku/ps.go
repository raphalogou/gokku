package dokku

import (
	"context"
	"fmt"
)

// Lifecycle actions on an app (APP-6). Each runs `dokku ps:<action> <app>`.
const (
	ActionRestart = "restart"
	ActionStop    = "stop"
	ActionStart   = "start"
	ActionRebuild = "rebuild"
)

// IsLifecycleAction reports whether a is one of the lifecycle actions.
func IsLifecycleAction(a string) bool {
	switch a {
	case ActionRestart, ActionStop, ActionStart, ActionRebuild:
		return true
	}
	return false
}

// Lifecycle runs ps:<action> for app and streams its output to onLine.
func (cl *Client) Lifecycle(ctx context.Context, action, app string, onLine func(Line)) error {
	if !IsLifecycleAction(action) {
		return fmt.Errorf("%w: action %q", ErrInvalidName, action)
	}
	if !ValidAppName(app) {
		return fmt.Errorf("%w: app %q", ErrInvalidName, app)
	}
	return cl.stream(ctx, newCmd("ps:"+action, app), nil, onLine)
}
