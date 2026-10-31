package api

import (
	"encoding/csv"
	"net/http"
	"strconv"
	"time"

	"gokku/internal/store"
)

func auditFilter(r *http.Request) (store.AuditFilter, error) {
	q := r.URL.Query()
	f := store.AuditFilter{Query: q.Get("q"), Actor: q.Get("user"), Project: q.Get("project"),
		Result: q.Get("result"), Source: q.Get("source")}
	fields := map[string]string{}
	if f.Result != "" && f.Result != "ok" && f.Result != "fail" {
		fields["result"] = "Use ok or fail."
	}
	switch f.Source {
	case "", "web", "api", "git", "system":
	default:
		fields["source"] = "Use web, api, git or system."
	}
	if c := q.Get("cursor"); c != "" {
		n, err := strconv.ParseInt(c, 10, 64)
		if err != nil || n <= 0 {
			fields["cursor"] = "Invalid cursor."
		}
		f.Cursor = n
	}
	if len(fields) > 0 {
		return f, validationError(fields)
	}
	return f, nil
}

// listAudit serves ACT-2: filtered entries, newest first, by cursor.
func (a *API) listAudit(w http.ResponseWriter, r *http.Request) error {
	f, err := auditFilter(r)
	if err != nil {
		return err
	}
	limit, err := pageLimit(r)
	if err != nil {
		return err
	}
	f.Limit = limit + 1
	entries, err := a.db.FindAudit(r.Context(), f)
	if err != nil {
		return err
	}
	actors, err := a.db.AuditActors(r.Context())
	if err != nil {
		return err
	}
	page := AuditPage{Items: []ActivityEntry{}, Actors: actors}
	if len(entries) > limit {
		entries = entries[:limit]
		next := strconv.FormatInt(entries[limit-1].ID, 10)
		page.NextCursor = &next
	}
	for _, e := range entries {
		page.Items = append(page.Items, toActivity(e))
	}
	writeJSON(w, http.StatusOK, page)
	return nil
}

// maxExportRows bounds one CSV export.
const maxExportRows = 100_000

// exportAudit serves the filtered audit log as CSV for admins (ACT-2). Details are already
// redacted when they are written (SEC-5).
func (a *API) exportAudit(w http.ResponseWriter, r *http.Request) error {
	f, err := auditFilter(r)
	if err != nil {
		return err
	}
	f.Cursor, f.Limit = 0, maxExportRows
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition",
		`attachment; filename="gokku-audit-`+time.Now().UTC().Format("2006-01-02")+`.csv"`)
	w.Header().Set("Cache-Control", "no-store")
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"id", "time", "actor", "actor_type", "action", "target_type", "target",
		"project", "result", "source", "job_id", "detail"})
	err = a.db.EachAudit(r.Context(), f, func(e store.AuditEntry) error {
		job := ""
		if e.JobID != nil {
			job = *e.JobID
		}
		return cw.Write([]string{strconv.FormatInt(e.ID, 10), e.TS.Format(time.RFC3339), csvSafe(e.ActorName),
			e.ActorType, csvSafe(e.Action), e.TargetType, csvSafe(e.TargetName), csvSafe(e.ProjectSlug),
			e.Result, e.Source, job, csvSafe(e.Detail)})
	})
	cw.Flush()
	if err != nil {
		a.log.Error("audit export", "error", err)
	}
	return nil
}

// csvSafe defuses spreadsheet formulas in exported text.
func csvSafe(s string) string {
	if s != "" && (s[0] == '=' || s[0] == '+' || s[0] == '-' || s[0] == '@') {
		return "'" + s
	}
	return s
}
