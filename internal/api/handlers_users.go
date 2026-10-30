package api

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"gokku/internal/audit"
	"gokku/internal/authz"
	"gokku/internal/store"
)

func (a *API) listUsers(w http.ResponseWriter, r *http.Request) error {
	users, err := a.db.ListUsers(r.Context())
	if err != nil {
		return err
	}
	out := make([]User, 0, len(users))
	for _, u := range users {
		out = append(out, toUser(u.User, u.LastActiveAt))
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (a *API) inviteUser(w http.ResponseWriter, r *http.Request) error {
	var req InviteUserRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	ctx := r.Context()
	u, token, err := a.auth.Invite(ctx, req.Username, string(req.Role))
	if errors.Is(err, store.ErrConflict) {
		return usernameTaken()
	}
	if err != nil {
		return err
	}
	a.audit.Record(ctx, authz.FromContext(ctx), audit.Entry{
		Action: "users:invite", TargetType: "user", TargetName: u.Username, Detail: map[string]any{"role": u.Role},
	})
	writeJSON(w, http.StatusCreated, a.inviteCreated(r, u, token))
	return nil
}

func (a *API) resetInvite(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	u, token, err := a.auth.ResetInvite(ctx, chi.URLParam(r, "id"))
	if err != nil {
		return err
	}
	a.audit.Record(ctx, authz.FromContext(ctx), audit.Entry{Action: "users:invite-reset", TargetType: "user", TargetName: u.Username})
	writeJSON(w, http.StatusOK, a.inviteCreated(r, u, token))
	return nil
}

func (a *API) inviteCreated(r *http.Request, u store.User, token string) InviteCreated {
	return InviteCreated{
		User:      toUser(u, nil),
		InviteUrl: baseURL(r) + "/invite/" + token,
		ExpiresAt: u.InviteExpiresAt.Time,
	}
}

func (a *API) updateUser(w http.ResponseWriter, r *http.Request) error {
	var req UpdateUserRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	ctx := r.Context()
	p := authz.FromContext(ctx)
	before, err := a.db.UserByID(ctx, chi.URLParam(r, "id"))
	if err != nil {
		return err
	}

	detail := map[string]any{}
	record := func(result string) {
		if len(detail) > 0 {
			a.audit.Record(ctx, p, audit.Entry{
				Action: "users:update", TargetType: "user", TargetName: before.Username, Result: result, Detail: detail,
			})
		}
	}
	if req.Role != nil && string(*req.Role) != before.Role {
		detail["role"] = map[string]any{"from": before.Role, "to": *req.Role}
		if err := a.auth.SetRole(ctx, before.ID, string(*req.Role)); err != nil {
			record(audit.ResultFail)
			return err
		}
	}
	if req.Status != nil && string(*req.Status) != before.Status {
		detail["status"] = map[string]any{"from": before.Status, "to": *req.Status}
		if err := a.auth.SetActive(ctx, before.ID, *req.Status == UpdateUserRequestStatusActive); err != nil {
			record(audit.ResultFail)
			return err
		}
		if *req.Status == UpdateUserRequestStatusDisabled {
			a.hub.CloseUser(before.ID) // EVT-2
		}
	}
	record(audit.ResultOK)

	users, err := a.db.ListUsers(ctx)
	if err != nil {
		return err
	}
	for _, u := range users {
		if u.ID == before.ID {
			writeJSON(w, http.StatusOK, toUser(u.User, u.LastActiveAt))
			return nil
		}
	}
	return store.ErrNotFound
}

func (a *API) deleteUser(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	p := authz.FromContext(ctx)
	u, err := a.db.UserByID(ctx, chi.URLParam(r, "id"))
	if err != nil {
		return err
	}
	entry := audit.Entry{Action: "users:delete", TargetType: "user", TargetName: u.Username}
	if err := a.auth.DeleteUser(ctx, u.ID); err != nil {
		entry.Result = audit.ResultFail
		a.audit.Record(ctx, p, entry)
		return err
	}
	a.audit.Record(ctx, p, entry)
	a.hub.CloseUser(u.ID) // EVT-2
	w.WriteHeader(http.StatusNoContent)
	return nil
}
