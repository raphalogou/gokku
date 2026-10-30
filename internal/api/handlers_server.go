package api

import (
	"net/http"
	"slices"

	"gokku/internal/audit"
	"gokku/internal/authz"
	"gokku/internal/dokku"
	"gokku/internal/snapshot"
)

// getServer serves SRV-1 from the snapshot.
func (a *API) getServer(w http.ResponseWriter, r *http.Request) error {
	writeJSON(w, http.StatusOK, a.serverInfo(r, a.snap.State().Server))
	return nil
}

// rescanServer re-runs the capability probe (PLG-2). An unreachable Dokku is reported in
// the response, not as an error.
func (a *API) rescanServer(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	entry := audit.Entry{Action: "server:rescan", TargetType: "server", TargetName: a.serverName}
	if err := a.snap.Rescan(ctx); err != nil {
		entry.Result = audit.ResultFail
	}
	a.audit.Record(ctx, authz.FromContext(ctx), entry)
	writeJSON(w, http.StatusOK, a.serverInfo(r, a.snap.State().Server))
	return nil
}

func (a *API) serverInfo(r *http.Request, s snapshot.Server) ServerInfo {
	caps := s.Capabilities
	out := ServerInfo{
		Name:        a.serverName,
		Host:        a.dokkuHost,
		Reachable:   s.Reachable,
		LastOkAt:    timePtr(s.LastOKAt),
		Plugins:     []string{},
		Datastores:  nonNil(caps.Datastores),
		Letsencrypt: caps.Letsencrypt,
		Unavailable: map[string]string{},
	}
	if out.Host == "" {
		out.Host = requestHost(r)
	}
	if s.Probed {
		out.DokkuVersion = &caps.DokkuVersion
	}
	if s.Error != "" {
		out.Error = &s.Error
	}
	if s.ErrorCommand != "" {
		out.ErrorCommand = &s.ErrorCommand
	}
	for _, p := range caps.Plugins {
		if p.Enabled {
			out.Plugins = append(out.Plugins, p.Name)
		}
	}
	for k, v := range caps.Unavailable {
		out.Unavailable[k] = v
	}
	return out
}

// recognisedPlugins are the plugins Gokku knows how to use, for the "not installed" list
// with its install command (PLG-1).
var recognisedPlugins = func() []AvailablePlugin {
	var out []AvailablePlugin
	for _, name := range dokku.Datastores {
		out = append(out, AvailablePlugin{Name: name, Kind: PluginKindDatastore, GitUrl: "https://github.com/dokku/dokku-" + name + ".git"})
	}
	return append(out, AvailablePlugin{Name: "letsencrypt", Kind: PluginKindLetsencrypt, GitUrl: "https://github.com/dokku/dokku-letsencrypt.git"})
}()

func pluginKind(name string) PluginKind {
	switch {
	case dokku.IsDatastore(name):
		return PluginKindDatastore
	case name == "letsencrypt":
		return PluginKindLetsencrypt
	default:
		return PluginKindOther
	}
}

// listPlugins serves PLG-1: installed plugins that are not part of Dokku core, and the
// recognised plugins that are not installed.
func (a *API) listPlugins(w http.ResponseWriter, _ *http.Request) error {
	caps := a.snap.State().Server.Capabilities
	out := PluginList{Installed: []InstalledPlugin{}, Available: []AvailablePlugin{}}
	installed := map[string]bool{}
	for _, p := range caps.Plugins {
		installed[p.Name] = true
		// Core plugins carry Dokku's own version.
		if p.Version == caps.DokkuVersion {
			continue
		}
		ip := InstalledPlugin{Name: p.Name, Version: p.Version, Kind: pluginKind(p.Name)}
		if reason, ok := caps.Unavailable[p.Name]; ok {
			ip.Reason = &reason
		} else {
			ip.Usable = ip.Kind != PluginKindOther
		}
		out.Installed = append(out.Installed, ip)
	}
	for _, p := range recognisedPlugins {
		if !installed[p.Name] {
			out.Available = append(out.Available, p)
		}
	}
	slices.SortStableFunc(out.Installed, func(x, y InstalledPlugin) int {
		return kindOrder(x.Kind) - kindOrder(y.Kind)
	})
	writeJSON(w, http.StatusOK, out)
	return nil
}

func kindOrder(k PluginKind) int {
	switch k {
	case PluginKindDatastore:
		return 0
	case PluginKindLetsencrypt:
		return 1
	default:
		return 2
	}
}
