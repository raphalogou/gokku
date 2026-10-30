package dokku

import (
	"context"
	"slices"
	"strings"
)

// Service is one datastore service as reported by `<type>:info`.
type Service struct {
	Type    string
	Name    string
	Status  string // docker state of the service container: running, exited, paused, missing, ...
	Version string // image and tag, such as postgres:17.6
	Links   []string
}

// Services returns every service of a datastore plugin with one bulk `<type>:info`
// command. This needs the datastore plugins built on dokku-datastore (2.6.0 or later),
// which report every service when no name is given; see docs/COMPATIBILITY.md.
//
// The report also carries the connection string. Only the fields above are kept: the
// output is never logged or stored.
func (cl *Client) Services(ctx context.Context, typ string) ([]Service, error) {
	if !IsDatastore(typ) {
		return nil, ErrInvalidName
	}
	out, err := cl.run(ctx, newCmd(typ+":info"))
	if err != nil {
		return nil, err
	}
	return parseServices(out, typ)
}

func parseServices(out, typ string) ([]Service, error) {
	sections, err := parseReport(out, typ)
	if err != nil {
		return nil, err
	}
	services := make([]Service, 0, len(sections))
	for name, f := range sections {
		if !ValidServiceName(name) {
			continue
		}
		links := f.list("links", ",")
		slices.Sort(links)
		services = append(services, Service{
			Type:    typ,
			Name:    name,
			Status:  f["status"],
			Version: f["version"],
			Links:   links,
		})
	}
	slices.SortFunc(services, func(a, b Service) int { return strings.Compare(a.Name, b.Name) })
	return services, nil
}
