package api

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"gokku/internal/audit"
	"gokku/internal/authz"
	"gokku/internal/store"
)

func (a *API) listTokens(w http.ResponseWriter, r *http.Request) error {
	tokens, err := a.db.ListTokens(r.Context())
	if err != nil {
		return err
	}
	out := make([]Token, 0, len(tokens))
	for _, t := range tokens {
		out = append(out, toToken(t))
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (a *API) createToken(w http.ResponseWriter, r *http.Request) error {
	var req CreateTokenRequest
	if err := decode(r, &req); err != nil {
		return err
	}
	ctx := r.Context()
	p := authz.FromContext(ctx)
	item, secret, err := a.auth.CreateToken(ctx, p.UserID, req.Name, string(req.Role))
	if errors.Is(err, store.ErrConflict) {
		ae := newError(http.StatusConflict, ErrorCodeTokenNameTaken, "You already have a token with this name.")
		ae.Fields = map[string]string{"name": "You already have a token with this name."}
		return ae
	}
	if err != nil {
		return err
	}
	a.audit.Record(ctx, p, audit.Entry{
		Action: "tokens:create", TargetType: "token", TargetName: item.Name, Detail: map[string]any{"role": item.Role},
	})
	writeJSON(w, http.StatusCreated, TokenCreated{Token: toToken(item), Secret: secret})
	return nil
}

func (a *API) revokeToken(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	t, err := a.db.TokenListItemByID(ctx, chi.URLParam(r, "id"))
	if err != nil {
		return err
	}
	if err := a.db.RevokeToken(ctx, t.ID); err != nil {
		return err
	}
	a.audit.Record(ctx, authz.FromContext(ctx), audit.Entry{
		Action: "tokens:revoke", TargetType: "token", TargetName: t.Name, Detail: map[string]any{"owner": t.Owner},
	})
	w.WriteHeader(http.StatusNoContent)
	return nil
}
