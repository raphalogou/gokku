package api

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"gokku/internal/audit"
	"gokku/internal/auth"
	"gokku/internal/authz"
)

func (a *API) getSetupStatus(w http.ResponseWriter, r *http.Request) error {
	required, err := a.auth.SetupRequired(r.Context())
	if err != nil {
		return err
	}
	minLen, err := a.auth.MinPasswordLength(r.Context())
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, SetupStatus{SetupRequired: required, MinPasswordLength: minLen})
	return nil
}

func (a *API) completeSetup(w http.ResponseWriter, r *http.Request) error {
	var req Credentials
	if err := decode(r, &req); err != nil {
		return err
	}
	u, sess, err := a.auth.Setup(r.Context(), req.Username, req.Password, clientInfo(r))
	if err != nil {
		return err
	}
	setSessionCookie(w, r, sess)
	a.audit.Record(r.Context(), principalFor(u.ID, u.Username), audit.Entry{
		Action: "setup:complete", TargetType: "user", TargetName: u.Username,
	})
	writeJSON(w, http.StatusCreated, toMe(u))
	return nil
}

func (a *API) login(w http.ResponseWriter, r *http.Request) error {
	var req Credentials
	if err := decode(r, &req); err != nil {
		return err
	}
	u, sess, err := a.auth.Login(r.Context(), req.Username, req.Password, clientInfo(r))
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) || errors.Is(err, auth.ErrRateLimited) {
			a.audit.Record(r.Context(), nil, audit.Entry{
				Action: "auth:login", Result: audit.ResultFail, ActorName: truncateName(req.Username),
				Detail: map[string]any{"ip": requestInfo(r).ClientIP, "reason": toAPIError(err).Code},
			})
		}
		return err
	}
	setSessionCookie(w, r, sess)
	a.audit.Record(r.Context(), principalFor(u.ID, u.Username), audit.Entry{
		Action: "auth:login", Detail: map[string]any{"ip": requestInfo(r).ClientIP},
	})
	writeJSON(w, http.StatusOK, toMe(u))
	return nil
}

func (a *API) logout(w http.ResponseWriter, r *http.Request) error {
	if c, err := r.Cookie(SessionCookie); err == nil {
		if err := a.auth.Logout(r.Context(), c.Value); err != nil {
			return err
		}
	}
	if p := authz.FromContext(r.Context()); p != nil {
		a.audit.Record(r.Context(), p, audit.Entry{Action: "auth:logout"})
	}
	clearSessionCookie(w, r)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (a *API) getInvite(w http.ResponseWriter, r *http.Request) error {
	u, err := a.auth.LookupInvite(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, Invite{Username: u.Username, ExpiresAt: u.InviteExpiresAt.Time})
	return nil
}

func (a *API) acceptInvite(w http.ResponseWriter, r *http.Request) error {
	var req AcceptInviteRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	u, sess, err := a.auth.AcceptInvite(r.Context(), req.Token, req.Password, clientInfo(r))
	if err != nil {
		return err
	}
	setSessionCookie(w, r, sess)
	a.audit.Record(r.Context(), principalFor(u.ID, u.Username), audit.Entry{
		Action: "users:accept-invite", TargetType: "user", TargetName: u.Username,
	})
	writeJSON(w, http.StatusOK, toMe(u))
	return nil
}

// principalFor is the actor of a request that just signed someone in.
func principalFor(id, username string) *authz.Principal {
	return &authz.Principal{UserID: id, Username: username, Kind: authz.KindSession}
}

// truncateName bounds an attacker-supplied username before it is stored in the audit log.
func truncateName(s string) string {
	if len(s) > 64 {
		return s[:64]
	}
	return s
}
