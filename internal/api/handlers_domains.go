package api

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"gokku/internal/audit"
	"gokku/internal/authz"
	"gokku/internal/dokku"
	"gokku/internal/jobs"
	"gokku/internal/snapshot"
	"gokku/internal/store"
)

// certificateLifetime is how long before its expiry a Let's Encrypt certificate was issued:
// 90 days of validity from a notBefore backdated by one hour (SSL-2).
const certificateLifetime = 90*24*time.Hour - time.Hour

func (a *API) appDomains(r *http.Request, domains []string) AppDomains {
	host := a.dokkuHost
	if host == "" {
		host = requestHost(r)
	}
	return AppDomains{Domains: nonNil(domains), DnsTarget: host}
}

// domainsTarget returns the app of a domains or SSL route.
func (a *API) domainsTarget(r *http.Request, needDokku bool) (snapshot.App, error) {
	state := a.snap.State()
	app, ok := state.App(chi.URLParam(r, "name"))
	if !ok {
		return app, newError(http.StatusNotFound, ErrorCodeNotFound, "No app with this name on the server.")
	}
	if needDokku && !state.Server.Reachable {
		return app, errDokkuUnreachable
	}
	return app, nil
}

func (a *API) getAppDomains(w http.ResponseWriter, r *http.Request) error {
	app, err := a.domainsTarget(r, false)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, a.appDomains(r, app.Domains))
	return nil
}

// addAppDomain implements DOM-1.
func (a *API) addAppDomain(w http.ResponseWriter, r *http.Request) error {
	var req DomainRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	app, err := a.domainsTarget(r, true)
	if err != nil {
		return err
	}
	domain := strings.ToLower(strings.TrimSpace(req.Domain))
	if !dokku.ValidDomain(domain) {
		return validationError(map[string]string{"domain": "Use a host name such as app.example.com."})
	}
	if slices.Contains(app.Domains, domain) {
		ae := newError(http.StatusConflict, ErrorCodeDomainExists, "The app has this domain already.")
		ae.Fields = map[string]string{"domain": "The app has this domain already."}
		return ae
	}
	ctx := r.Context()
	err = a.dokku.AddDomain(ctx, app.Name, domain)
	a.recordDomain(ctx, "domains:add", app.Name, domain, err)
	if err != nil {
		return err
	}
	a.snap.Poke()
	writeJSON(w, http.StatusCreated, a.appDomains(r, append(slices.Clone(app.Domains), domain)))
	return nil
}

func (a *API) removeAppDomain(w http.ResponseWriter, r *http.Request) error {
	app, err := a.domainsTarget(r, true)
	if err != nil {
		return err
	}
	domain := r.URL.Query().Get("domain")
	if !slices.Contains(app.Domains, domain) {
		return newError(http.StatusNotFound, ErrorCodeNotFound, "The app does not have this domain.")
	}
	ctx := r.Context()
	err = a.dokku.RemoveDomain(ctx, app.Name, domain)
	a.recordDomain(ctx, "domains:remove", app.Name, domain, err)
	if err != nil {
		return err
	}
	a.snap.Poke()
	writeJSON(w, http.StatusOK, a.appDomains(r, slices.DeleteFunc(slices.Clone(app.Domains), func(d string) bool { return d == domain })))
	return nil
}

func (a *API) recordDomain(ctx context.Context, action, app, domain string, err error) {
	_, slug, _ := a.appProject(ctx, app)
	e := audit.Entry{Action: action, TargetType: "app", TargetName: app, ProjectSlug: slug,
		Detail: map[string]any{"domain": domain}}
	if err != nil {
		e.Result = audit.ResultFail
	}
	a.audit.Record(ctx, authz.FromContext(ctx), e)
}

// sslStatus reads the app's Let's Encrypt report (SSL-1) and the domains added since the
// certificate was issued (SSL-2).
func (a *API) sslStatus(ctx context.Context, app snapshot.App) (SSLStatus, error) {
	out := SSLStatus{Uncovered: []string{}}
	if !a.snap.State().Server.Capabilities.Letsencrypt {
		return out, nil
	}
	out.Available = true
	rep, err := a.dokku.Letsencrypt(ctx, app.Name)
	if err != nil {
		return out, err
	}
	out.Active, out.AutoRenew, out.ExpiresAt = rep.Active, rep.AutoRenew, timePtr(rep.Expires)
	if rep.ComputedEmail != "" {
		out.Email = &rep.ComputedEmail
	}
	if rep.Active && !rep.Expires.IsZero() {
		// TODO(spec-2): Dokku issues one certificate per app and does not say which domains
		// it covers; until it does, warn about domains added after the last issuance.
		// Expiry times have whole seconds; allow one for the issue time it gives.
		issued := rep.Expires.Add(-certificateLifetime + time.Second)
		added, err := a.db.DomainsAddedSince(ctx, app.Name, store.At(issued))
		if err != nil {
			return out, err
		}
		for _, d := range added {
			if slices.Contains(app.Domains, d) {
				out.Uncovered = append(out.Uncovered, d)
			}
		}
		slices.Sort(out.Uncovered)
	}
	return out, nil
}

func (a *API) getAppSSL(w http.ResponseWriter, r *http.Request) error {
	app, err := a.domainsTarget(r, true)
	if err != nil {
		return err
	}
	out, err := a.sslStatus(r.Context(), app)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// changeAppSSL implements SSL-1: certificate requests run as jobs, settings directly.
func (a *API) changeAppSSL(w http.ResponseWriter, r *http.Request) error {
	var req SSLRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	app, err := a.domainsTarget(r, true)
	if err != nil {
		return err
	}
	if !a.snap.State().Server.Capabilities.Letsencrypt {
		return newError(http.StatusConflict, ErrorCodePluginMissing, "The letsencrypt plugin is not installed on this server.")
	}
	ctx := r.Context()
	_, slug, _ := a.appProject(ctx, app.Name)
	actor := authz.FromContext(ctx)
	entry := audit.Entry{Action: "ssl:" + strings.ReplaceAll(string(req.Action), "_", "-"), TargetType: "app",
		TargetName: app.Name, ProjectSlug: slug}

	switch req.Action {
	case SSLRequestActionEnable, SSLRequestActionRenew, SSLRequestActionDisable:
		if req.Action != SSLRequestActionDisable && len(app.Domains) == 0 {
			return validationError(map[string]string{"action": "Add a domain before enabling SSL."})
		}
		title := map[SSLRequestAction]string{
			SSLRequestActionEnable: "Enable SSL", SSLRequestActionRenew: "Renew SSL", SSLRequestActionDisable: "Disable SSL",
		}[req.Action]
		job, err := a.jobs.Submit(ctx, jobs.Spec{
			Kind: "app.ssl", Title: title + " for " + app.Name, App: app.Name, Actor: actor, Steps: []string{title},
			Run: func(ctx context.Context, t *jobs.Task) error {
				if req.Action == SSLRequestActionDisable {
					return a.dokku.DisableLetsencrypt(ctx, app.Name, t.Line)
				}
				return a.dokku.EnableLetsencrypt(ctx, app.Name, req.Action == SSLRequestActionRenew, t.Line)
			},
			Finished: func(ctx context.Context, job store.Job) { a.auditJob(ctx, actor, job, entry) },
		})
		if err != nil {
			return err
		}
		writeJSON(w, http.StatusAccepted, JobRef{JobId: job.ID})
		return nil
	case SSLRequestActionSetEmail:
		email := ""
		if req.Email != nil {
			email = strings.TrimSpace(*req.Email)
		}
		err = a.dokku.SetLetsencryptEmail(ctx, app.Name, email)
		if err != nil && strings.Contains(err.Error(), "email") {
			return validationError(map[string]string{"email": "Use an email address such as ops@example.com."})
		}
		entry.Detail = map[string]any{"email": email}
	case SSLRequestActionSetAutoRenew:
		if req.AutoRenew == nil {
			return validationError(map[string]string{"auto_renew": "Required."})
		}
		err = a.dokku.SetLetsencryptAutoRenew(ctx, *req.AutoRenew)
		entry.TargetType, entry.TargetName, entry.ProjectSlug = "server", a.serverName, ""
		entry.Detail = map[string]any{"auto_renew": *req.AutoRenew}
	default:
		return validationError(map[string]string{"action": "Unknown action."})
	}
	if err != nil {
		entry.Result = audit.ResultFail
	}
	a.audit.Record(ctx, actor, entry)
	if err != nil {
		return err
	}
	out, err := a.sslStatus(ctx, app)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}
