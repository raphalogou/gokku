package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// listApps serves APP-1 from the snapshot.
func (a *API) listApps(w http.ResponseWriter, r *http.Request) error {
	v, err := a.loadView(r.Context())
	if err != nil {
		return err
	}
	out := make([]AppSummary, 0, len(v.state.Apps))
	for _, app := range v.state.Apps {
		out = append(out, v.appSummary(app))
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (a *API) getApp(w http.ResponseWriter, r *http.Request) error {
	v, err := a.loadView(r.Context())
	if err != nil {
		return err
	}
	app, ok := v.state.App(chi.URLParam(r, "name"))
	if !ok {
		return newError(http.StatusNotFound, ErrorCodeNotFound, "No app with this name on the server.")
	}
	writeJSON(w, http.StatusOK, v.appDetail(app))
	return nil
}

// listServices serves the read-only services list (SVC-2) from the snapshot.
func (a *API) listServices(w http.ResponseWriter, r *http.Request) error {
	v, err := a.loadView(r.Context())
	if err != nil {
		return err
	}
	out := make([]ServiceSummary, 0, len(v.state.Services))
	for _, s := range v.state.Services {
		out = append(out, v.serviceSummary(s))
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}
