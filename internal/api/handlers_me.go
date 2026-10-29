package api

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"gokku/internal/audit"
	"gokku/internal/authz"
	"gokku/internal/store"
)

func (a *API) getMe(w http.ResponseWriter, r *http.Request) error {
	u, err := a.db.UserByID(r.Context(), authz.FromContext(r.Context()).UserID)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, toMe(u))
	return nil
}

func (a *API) updateMe(w http.ResponseWriter, r *http.Request) error {
	var req UpdateMeRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	ctx := r.Context()
	p := authz.FromContext(ctx)

	if req.Username != nil && *req.Username != p.Username {
		old := p.Username
		if err := a.auth.Rename(ctx, p.UserID, *req.Username); err != nil {
			if errors.Is(err, store.ErrConflict) {
				return usernameTaken()
			}
			return err
		}
		a.audit.Record(ctx, p, audit.Entry{
			Action: "users:rename", TargetType: "user", TargetName: *req.Username,
			Detail: map[string]any{"old": old, "new": *req.Username},
		})
	}
	if req.Theme != nil {
		if err := a.auth.SetTheme(ctx, p.UserID, string(*req.Theme)); err != nil {
			return err
		}
	}

	u, err := a.db.UserByID(ctx, p.UserID)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, toMe(u))
	return nil
}

func (a *API) changePassword(w http.ResponseWriter, r *http.Request) error {
	var req ChangePasswordRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	ctx := r.Context()
	p := authz.FromContext(ctx)
	if err := a.auth.ChangePassword(ctx, p.UserID, p.SessionID, req.CurrentPassword, req.NewPassword); err != nil {
		return err
	}
	a.audit.Record(ctx, p, audit.Entry{Action: "users:password", TargetType: "user", TargetName: p.Username})
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (a *API) listMySessions(w http.ResponseWriter, r *http.Request) error {
	p := authz.FromContext(r.Context())
	sessions, err := a.db.ListUserSessions(r.Context(), p.UserID)
	if err != nil {
		return err
	}
	out := make([]Session, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, Session{
			Id:         s.ID,
			CreatedAt:  s.CreatedAt.Time,
			LastSeenAt: s.LastSeenAt.Time,
			ExpiresAt:  s.ExpiresAt.Time,
			UserAgent:  s.UserAgent,
			Ip:         s.IP,
			Current:    s.ID == p.SessionID,
		})
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (a *API) revokeMySession(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	p := authz.FromContext(ctx)
	id := chi.URLParam(r, "id")
	if id == p.SessionID {
		return newError(http.StatusBadRequest, ErrorCodeCannotRevokeCurrent, "Use Log out to end the current session.")
	}
	if err := a.db.DeleteSession(ctx, p.UserID, id); err != nil {
		return err
	}
	a.audit.Record(ctx, p, audit.Entry{Action: "sessions:revoke", TargetType: "user", TargetName: p.Username})
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func usernameTaken() *apiError {
	ae := newError(http.StatusConflict, ErrorCodeUsernameTaken, "This username is taken.")
	ae.Fields = map[string]string{"username": "This username is taken."}
	return ae
}
