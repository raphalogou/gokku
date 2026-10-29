// Package audit records every mutation (ACT-1).
package audit

import (
	"context"
	"encoding/json"
	"log/slog"
	"regexp"

	"gokku/internal/authz"
	"gokku/internal/store"
)

// Results.
const (
	ResultOK   = "ok"
	ResultFail = "fail"
)

// Entry describes one audited action. Detail must never hold secret values; keys that look
// secret are redacted as a safety net (SEC-5).
type Entry struct {
	Action      string // e.g. users:invite, config:set
	TargetType  string
	TargetName  string
	ProjectSlug string
	Result      string // ResultOK unless set
	JobID       string
	Detail      map[string]any
	// ActorName is used when there is no principal, for example a failed sign-in.
	ActorName string
}

// Recorder writes audit entries.
type Recorder struct {
	db  *store.DB
	log *slog.Logger
}

// New returns a Recorder.
func New(db *store.DB, log *slog.Logger) *Recorder {
	return &Recorder{db: db, log: log}
}

// Record writes e on behalf of p (nil for anonymous or system actions). A failure to write
// is logged, not returned: the audited operation has already happened.
func (r *Recorder) Record(ctx context.Context, p *authz.Principal, e Entry) {
	row := store.AuditEntry{
		TS:          store.Now(),
		Action:      e.Action,
		TargetType:  e.TargetType,
		TargetName:  e.TargetName,
		ProjectSlug: e.ProjectSlug,
		Result:      e.Result,
		ActorType:   "user",
		ActorName:   e.ActorName,
		Source:      "web",
	}
	if row.Result == "" {
		row.Result = ResultOK
	}
	if e.JobID != "" {
		row.JobID = &e.JobID
	}
	switch {
	case p != nil:
		id := p.UserID
		row.ActorID = &id
		row.ActorName = p.Username
		if p.Kind == authz.KindToken {
			row.ActorType = "token"
			row.Source = "api"
		}
	case e.ActorName == "":
		row.ActorType = "system"
		row.ActorName = "system"
		row.Source = "system"
	}

	detail, err := json.Marshal(Redact(e.Detail))
	if err != nil || e.Detail == nil {
		detail = []byte("{}")
	}
	row.Detail = string(detail)
	if p != nil && p.Kind == authz.KindToken {
		row.Detail = mergeToken(row.Detail, p.TokenName)
	}

	if err := r.db.InsertAudit(ctx, row); err != nil {
		r.log.LogAttrs(ctx, slog.LevelError, "write audit entry", slog.String("action", e.Action), slog.Any("error", err))
	}
}

// RecordSystem writes an entry for an action Gokku or a server command performed.
func (r *Recorder) RecordSystem(ctx context.Context, e Entry) {
	e.ActorName = ""
	r.Record(ctx, nil, e)
}

// secretKey matches keys whose values must never be stored (the CFG-1 pattern).
var secretKey = regexp.MustCompile(`(?i)(secret|token|password|passwd|key|dsn|url|credential)`)

// Redact returns a copy of detail with values under secret-looking keys replaced.
func Redact(detail map[string]any) map[string]any {
	if detail == nil {
		return nil
	}
	out := make(map[string]any, len(detail))
	for k, v := range detail {
		switch {
		case secretKey.MatchString(k):
			out[k] = "[redacted]"
		default:
			if m, ok := v.(map[string]any); ok {
				v = Redact(m)
			}
			out[k] = v
		}
	}
	return out
}

func mergeToken(detail, tokenName string) string {
	var m map[string]any
	if json.Unmarshal([]byte(detail), &m) != nil || m == nil {
		m = map[string]any{}
	}
	m["via_token"] = tokenName
	b, err := json.Marshal(m)
	if err != nil {
		return detail
	}
	return string(b)
}
