package api

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gokku/internal/auth"
	"gokku/internal/authz"
)

// SessionCookie is the name of the session cookie (AUTH-4).
const SessionCookie = "gokku_session"

// authenticate resolves the bearer token or session cookie into a principal. A request with
// an invalid bearer token fails at once; an invalid cookie leaves the request anonymous, so
// public routes still work and protected ones return 401.
func (a *API) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if h := r.Header.Get("Authorization"); h != "" {
			token, ok := strings.CutPrefix(h, "Bearer ")
			if !ok {
				writeError(w, errUnauthorized)
				return
			}
			p, err := a.auth.AuthenticateToken(ctx, strings.TrimSpace(token))
			if err != nil {
				a.writeAuthError(w, r, err)
				return
			}
			next.ServeHTTP(w, r.WithContext(authz.WithPrincipal(ctx, p)))
			return
		}

		c, err := r.Cookie(SessionCookie)
		if err != nil || c.Value == "" {
			next.ServeHTTP(w, r)
			return
		}
		res, err := a.auth.AuthenticateSession(ctx, c.Value)
		if errors.Is(err, auth.ErrUnauthenticated) {
			clearSessionCookie(w, r)
			next.ServeHTTP(w, r)
			return
		}
		if err != nil {
			a.writeAuthError(w, r, err)
			return
		}
		if res.Refreshed != nil {
			setSessionCookie(w, r, *res.Refreshed)
		}
		next.ServeHTTP(w, r.WithContext(authz.WithPrincipal(ctx, res.Principal)))
	})
}

func (a *API) writeAuthError(w http.ResponseWriter, r *http.Request, err error) {
	if !errors.Is(err, auth.ErrUnauthenticated) {
		a.log.ErrorContext(r.Context(), "authenticate", "error", err)
	}
	writeError(w, toAPIError(err))
}

// csrf requires unsafe requests that rely on the cookie to be same-origin (AUTH-5): the
// Origin host must equal the request host, or Sec-Fetch-Site must be same-origin or none.
// Bearer-token requests are exempt because browsers never attach them on their own.
func csrf(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		if r.Header.Get("Authorization") != "" || sameOrigin(r) {
			next.ServeHTTP(w, r)
			return
		}
		writeError(w, newError(http.StatusForbidden, ErrorCodeCsrfFailed, "Cross-site request refused."))
	})
}

func sameOrigin(r *http.Request) bool {
	if origin := r.Header.Get("Origin"); origin != "" && origin != "null" {
		u, err := url.Parse(origin)
		return err == nil && u.Host != "" && strings.EqualFold(u.Host, r.Host)
	}
	switch r.Header.Get("Sec-Fetch-Site") {
	case "same-origin", "none":
		return true
	}
	return false
}

// Secure follows the request scheme (AUTH-4): set over HTTPS, directly or via a trusted proxy.
func setSessionCookie(w http.ResponseWriter, r *http.Request, s auth.NewSession) {
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // Secure is conditional by design, see above
		Name:     SessionCookie,
		Value:    s.Token,
		Path:     "/",
		Expires:  s.ExpiresAt,
		MaxAge:   int(time.Until(s.ExpiresAt).Seconds()),
		HttpOnly: true,
		Secure:   requestInfo(r).HTTPS,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // as setSessionCookie
		Name:     SessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   requestInfo(r).HTTPS,
		SameSite: http.SameSiteLaxMode,
	})
}
