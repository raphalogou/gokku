package api

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"gokku/internal/audit"
	"gokku/internal/auth"
	"gokku/internal/authz"
	"gokku/internal/dokku"
	"gokku/internal/jobs"
	"gokku/internal/store"
)

// getSettings serves SET-1: Gokku's settings, the protected apps (SET-2) and Dokku's global
// domains.
func (a *API) getSettings(w http.ResponseWriter, r *http.Request) error {
	out, err := a.settings(r.Context())
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (a *API) settings(ctx context.Context) (Settings, error) {
	out := Settings{GokkuVersion: a.version, ProtectedApps: []string{}}
	var err error
	if out.SessionLifetimeHours, err = a.db.SettingInt(ctx, store.SettingSessionLifetimeHours, auth.DefaultSessionLifetimeHours); err != nil {
		return out, err
	}
	if out.MinPasswordLength, err = a.db.SettingInt(ctx, store.SettingMinPasswordLength, auth.DefaultMinPasswordLength); err != nil {
		return out, err
	}
	if out.JobRetentionDays, err = a.db.SettingInt(ctx, store.SettingJobRetentionDays, jobs.DefaultRetentionDays); err != nil {
		return out, err
	}
	protected, err := a.protectedApps(ctx)
	if err != nil {
		return out, err
	}
	out.ProtectedApps = slices.Sorted(func(yield func(string) bool) {
		for name := range protected {
			if !yield(name) {
				return
			}
		}
	})
	if a.snap.State().Server.Reachable {
		domains, err := a.dokku.GlobalDomains(ctx)
		if err != nil {
			a.log.Warn("global domains", "error", err)
		} else {
			out.GlobalDomains = &domains
		}
	}
	return out, nil
}

// updateSettings changes the given settings (SET-1, SET-2). Global domains are compared
// with Dokku's and only the differences are added or removed.
func (a *API) updateSettings(w http.ResponseWriter, r *http.Request) error {
	var req UpdateSettingsRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	fields := map[string]string{}
	inRange := func(field string, v *int, lo, hi int) {
		if v != nil && (*v < lo || *v > hi) {
			fields[field] = "Use a number from " + strconv.Itoa(lo) + " to " + strconv.Itoa(hi) + "."
		}
	}
	inRange("session_lifetime_hours", req.SessionLifetimeHours, 1, 8760)
	inRange("min_password_length", req.MinPasswordLength, 8, 128)
	inRange("job_retention_days", req.JobRetentionDays, 1, 3650)
	var protected, domains []string
	if req.ProtectedApps != nil {
		protected = normalizeNames(*req.ProtectedApps)
		for _, name := range protected {
			if !dokku.ValidAppName(name) {
				fields["protected_apps"] = name + " is not an app name."
				break
			}
		}
		if len(protected) > 200 {
			fields["protected_apps"] = "Protect at most 200 apps."
		}
	}
	if req.GlobalDomains != nil {
		domains = normalizeNames(*req.GlobalDomains)
		for _, d := range domains {
			if !dokku.ValidDomain(d) {
				fields["global_domains"] = d + " is not a domain name."
				break
			}
		}
		if len(domains) > 50 {
			fields["global_domains"] = "Use at most 50 global domains."
		}
	}
	if len(fields) > 0 {
		return validationError(fields)
	}

	ctx := r.Context()
	var added, removed []string
	if req.GlobalDomains != nil {
		if !a.snap.State().Server.Reachable {
			return errDokkuUnreachable
		}
		current, err := a.dokku.GlobalDomains(ctx)
		if err != nil {
			return err
		}
		for _, d := range domains {
			if !slices.Contains(current, d) {
				added = append(added, d)
			}
		}
		for _, d := range current {
			if !slices.Contains(domains, d) {
				removed = append(removed, d)
			}
		}
	}

	changed := []string{}
	for key, v := range map[string]*int{
		store.SettingSessionLifetimeHours: req.SessionLifetimeHours,
		store.SettingMinPasswordLength:    req.MinPasswordLength,
		store.SettingJobRetentionDays:     req.JobRetentionDays,
	} {
		if v != nil {
			if err := a.db.SetSetting(ctx, key, strconv.Itoa(*v)); err != nil {
				return err
			}
			changed = append(changed, key)
		}
	}
	if req.ProtectedApps != nil {
		raw, err := json.Marshal(protected)
		if err != nil {
			return err
		}
		if err := a.db.SetSetting(ctx, store.SettingProtectedApps, string(raw)); err != nil {
			return err
		}
		changed = append(changed, store.SettingProtectedApps)
	}

	actor := authz.FromContext(ctx)
	entry := audit.Entry{Action: "settings:update", TargetType: "server", TargetName: a.serverName,
		Detail: map[string]any{}}
	var domainErr error
	for _, d := range added {
		if domainErr = a.dokku.AddGlobalDomain(ctx, d); domainErr != nil {
			break
		}
	}
	for _, d := range removed {
		if domainErr != nil {
			break
		}
		domainErr = a.dokku.RemoveGlobalDomain(ctx, d)
	}
	slices.Sort(changed)
	entry.Detail["settings"] = changed
	if req.ProtectedApps != nil {
		entry.Detail["protected_apps"] = protected
	}
	if len(added) > 0 {
		entry.Detail["global_domains_added"] = added
	}
	if len(removed) > 0 {
		entry.Detail["global_domains_removed"] = removed
	}
	if domainErr != nil {
		entry.Result = audit.ResultFail
	}
	a.audit.Record(ctx, actor, entry)
	if domainErr != nil {
		return domainErr
	}
	out, err := a.settings(ctx)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// normalizeNames lowercases, trims and dedupes, keeping the order.
func normalizeNames(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.ToLower(strings.TrimSpace(s))
		if s != "" && !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}
