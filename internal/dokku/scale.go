package dokku

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
)

// processTypeRe is a Procfile process type; a leading dash is refused as it reads as a flag.
var processTypeRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_-]{0,62}$`)

// MaxScale is the most instances Gokku sets for one process type.
const MaxScale = 100

// Scale returns the desired instances per process type from
// `dokku ps:scale --format json <app>` (SCL-1).
func (cl *Client) Scale(ctx context.Context, app string) (map[string]int, error) {
	if !ValidAppName(app) {
		return nil, fmt.Errorf("%w: app %q", ErrInvalidName, app)
	}
	out, err := cl.run(ctx, newCmd("ps:scale", "--format", "json", app))
	if err != nil {
		return nil, err
	}
	return parseScale(out)
}

func parseScale(out string) (map[string]int, error) {
	var rows []struct {
		Type     string `json:"process_type"`
		Quantity int    `json:"quantity"`
	}
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		return nil, fmt.Errorf("ps:scale: %w", err)
	}
	scale := make(map[string]int, len(rows))
	for _, r := range rows {
		scale[r.Type] = r.Quantity
	}
	return scale, nil
}

// SetScale runs `dokku ps:scale <app> web=2 worker=1`, which applies the formation and
// restarts what changed, streaming the output.
func (cl *Client) SetScale(ctx context.Context, app string, scale map[string]int, onLine func(Line)) error {
	if !ValidAppName(app) {
		return fmt.Errorf("%w: app %q", ErrInvalidName, app)
	}
	if len(scale) == 0 {
		return nil
	}
	args := []string{"ps:scale", app}
	for _, typ := range slices.Sorted(maps.Keys(scale)) {
		n := scale[typ]
		if !processTypeRe.MatchString(typ) || n < 0 || n > MaxScale {
			return fmt.Errorf("%w: %s=%d", ErrInvalidName, typ, n)
		}
		args = append(args, typ+"="+strconv.Itoa(n))
	}
	return cl.stream(ctx, newCmd(args...), nil, onLine)
}
