package api

import (
	"bufio"
	"context"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"gokku/internal/store"
)

// waitJob polls GET /jobs/{id} until the job has finished.
func (c *client) waitJob(id string) JobDetail {
	c.e.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var j JobDetail
		res := c.do(http.MethodGet, "/jobs/"+id, nil)
		expect(c.e.t, res, http.StatusOK)
		res.decode(c.e.t, &j)
		if j.Status != JobStatusQueued && j.Status != JobStatusRunning {
			return j
		}
		time.Sleep(10 * time.Millisecond)
	}
	c.e.t.Fatalf("job %s did not finish", id)
	return JobDetail{}
}

func (c *client) startAction(app, action string) string {
	c.e.t.Helper()
	res := c.do(http.MethodPost, "/apps/"+app+"/actions/"+action, nil)
	expect(c.e.t, res, http.StatusAccepted)
	var ref JobRef
	res.decode(c.e.t, &ref)
	return ref.JobId
}

// APP-6, ACT-1: lifecycle actions run as jobs, change the app and are audited.
func TestAPP6LifecycleActions(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	operator, _ := e.member(admin, "olga", RoleOperator)
	viewer, _ := e.member(admin, "victor", RoleViewer)
	expect(t, admin.do(http.MethodPost, "/projects", CreateProjectRequest{Name: "Blog", Slug: "blog"}), http.StatusCreated)
	expect(t, admin.do(http.MethodPut, "/projects/blog/apps/blog-api", nil), http.StatusNoContent)

	expectError(t, viewer.do(http.MethodPost, "/apps/blog-api/actions/restart", nil), http.StatusForbidden, ErrorCodeForbidden)
	expectError(t, operator.do(http.MethodPost, "/apps/nope/actions/restart", nil), http.StatusNotFound, ErrorCodeNotFound)

	id := operator.startAction("blog-api", "stop")
	j := operator.waitJob(id)
	if j.Status != JobStatusSucceeded || j.Kind != "app.stop" || j.Title != "Stop blog-api" || j.App == nil ||
		j.Project == nil || j.Project.Slug != "blog" || j.Creator == nil || *j.Creator != "olga" ||
		len(j.Steps) != 1 || j.Steps[0].Status != StepStatusSucceeded {
		t.Errorf("job = %+v", j)
	}
	if err := e.snap.Refresh(t.Context()); err != nil {
		t.Fatal(err)
	}
	var app AppDetail
	viewer.do(http.MethodGet, "/apps/blog-api", nil).decode(t, &app)
	if app.Status != AppStatusStopped {
		t.Errorf("after stop: %s", app.Status)
	}
	a := e.lastAudit()
	if a.Action != "apps:stop" || a.ActorName != "olga" || a.Result != "ok" || a.JobID == nil || *a.JobID != id ||
		a.ProjectSlug != "blog" {
		t.Errorf("audit = %+v", a)
	}

	// A failing rebuild fails the job and is audited as a failure (ACT-1).
	id = operator.startAction("status-page", "rebuild")
	if j := operator.waitJob(id); j.Status != JobStatusFailed || j.Error == nil || !strings.Contains(*j.Error, "ps:rebuild") {
		t.Errorf("rebuild = %+v", j)
	}
	if a := e.lastAudit(); a.Action != "apps:rebuild" || a.Result != "fail" {
		t.Errorf("audit = %+v", a)
	}
	if err := e.snap.Refresh(t.Context()); err != nil {
		t.Fatal(err)
	}
	viewer.do(http.MethodGet, "/apps/status-page", nil).decode(t, &app)
	if app.Status != AppStatusFailed {
		t.Errorf("after a failed rebuild: %s (APP-2)", app.Status)
	}
}

// SET-2: stop and rebuild are refused for protected apps, for everyone.
func TestProtectedAppsRefuseStopAndRebuild(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	if err := e.db.SetSetting(t.Context(), store.SettingProtectedApps, `["blog-api"]`); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"stop", "rebuild"} {
		expectError(t, admin.do(http.MethodPost, "/apps/blog-api/actions/"+action, nil), http.StatusConflict, ErrorCodeAppProtected)
	}
	admin.waitJob(admin.startAction("blog-api", "restart"))
}

// SRV-2: actions are refused while Dokku is unreachable.
func TestSRV2ActionsNeedDokku(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	if err := os.WriteFile(e.fake.FailFile, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	_ = e.snap.Refresh(t.Context())
	expectError(t, admin.do(http.MethodPost, "/apps/blog-api/actions/restart", nil), http.StatusServiceUnavailable, ErrorCodeDokkuUnreachable)
	expectError(t, admin.do(http.MethodGet, "/apps/blog-api/logs", nil), http.StatusServiceUnavailable, ErrorCodeDokkuUnreachable)
}

// sseEvent is one parsed server-sent event.
type sseEvent struct{ id, name, data string }

// openSSE reads events from path until the stream ends or ctx is done.
func (c *client) openSSE(ctx context.Context, path string, header http.Header) (<-chan sseEvent, int) {
	c.e.t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.e.srv.URL+"/api/v1"+path, nil)
	if err != nil {
		c.e.t.Fatal(err)
	}
	for k, v := range header {
		req.Header[k] = v
	}
	res, err := c.http.Do(req) //nolint:bodyclose // the reader goroutine closes it
	if err != nil {
		c.e.t.Fatal(err)
	}
	out := make(chan sseEvent, 256)
	go func() {
		defer close(out)
		defer func() { _ = res.Body.Close() }()
		sc := bufio.NewScanner(res.Body)
		var ev sseEvent
		for sc.Scan() {
			line := sc.Text()
			switch {
			case line == "":
				if ev.name != "" || ev.data != "" {
					out <- ev
				}
				ev = sseEvent{}
			case strings.HasPrefix(line, ": "):
				out <- sseEvent{name: "comment", data: line[2:]}
			case strings.HasPrefix(line, "id: "):
				ev.id = line[4:]
			case strings.HasPrefix(line, "event: "):
				ev.name = line[7:]
			case strings.HasPrefix(line, "data: "):
				ev.data = line[6:]
			}
		}
	}()
	return out, res.StatusCode
}

func collect(t *testing.T, events <-chan sseEvent) []sseEvent {
	t.Helper()
	var all []sseEvent
	timeout := time.After(5 * time.Second)
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return all
			}
			all = append(all, ev)
		case <-timeout:
			t.Fatalf("stream did not end; got %d events", len(all))
		}
	}
}

// JOB-3, EVT-3: the job stream sends lines with their seq, steps and the final status,
// heartbeats while quiet, and resumes after Last-Event-ID.
func TestJOB3JobStream(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	e.fake.Delay = 60 * time.Millisecond
	id := admin.startAction("blog-api", "restart")

	events, status := admin.openSSE(t.Context(), "/jobs/"+id+"/stream", nil)
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	all := collect(t, events)
	var lines []JobLine
	var final JobDetail
	heartbeats := 0
	for _, ev := range all {
		switch ev.name {
		case "line":
			var l JobLine
			if err := json.Unmarshal([]byte(ev.data), &l); err != nil {
				t.Fatal(err)
			}
			if ev.id == "" || ev.id != strconv.FormatInt(l.Seq, 10) {
				t.Errorf("event id %q for seq %d", ev.id, l.Seq)
			}
			lines = append(lines, l)
		case "status":
			_ = json.Unmarshal([]byte(ev.data), &final)
		case "comment":
			if ev.data == "heartbeat" {
				heartbeats++
			}
		}
	}
	if len(lines) < 5 || lines[0].Seq != 1 || lines[0].Text != "-----> Restarting blog-api" || lines[0].Step == nil {
		t.Fatalf("lines = %+v", lines)
	}
	if final.Status != JobStatusSucceeded {
		t.Errorf("final = %+v", final)
	}
	if heartbeats == 0 {
		t.Error("no heartbeat while the job ran (EVT-3)")
	}

	// Resume after the third line from the stored output.
	events, _ = admin.openSSE(t.Context(), "/jobs/"+id+"/stream", http.Header{"Last-Event-ID": {"3"}})
	resumed := collect(t, events)
	if resumed[0].name != "comment" || resumed[1].id != "4" || resumed[len(resumed)-1].name != "status" {
		t.Errorf("resumed = %+v", resumed[:min(3, len(resumed))])
	}
	if n := len(resumed) - 2; n != len(lines)-3 {
		t.Errorf("resumed %d lines, want %d", n, len(lines)-3)
	}
	expectError(t, admin.do(http.MethodGet, "/jobs/nope", nil), http.StatusNotFound, ErrorCodeNotFound)
}

// JOB-4: cancel a running job; a finished job cannot be canceled.
func TestJOB4CancelJob(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	viewer, _ := e.member(admin, "victor", RoleViewer)
	e.fake.Delay = 200 * time.Millisecond
	id := admin.startAction("blog-api", "rebuild")
	expectError(t, viewer.do(http.MethodPost, "/jobs/"+id+"/cancel", nil), http.StatusForbidden, ErrorCodeForbidden)
	expect(t, admin.do(http.MethodPost, "/jobs/"+id+"/cancel", nil), http.StatusAccepted)
	if j := admin.waitJob(id); j.Status != JobStatusCanceled || j.Steps[0].Status != StepStatusCanceled {
		t.Errorf("job = %+v", j)
	}
	expectError(t, admin.do(http.MethodPost, "/jobs/"+id+"/cancel", nil), http.StatusConflict, ErrorCodeJobFinished)
	expectError(t, admin.do(http.MethodPost, "/jobs/nope/cancel", nil), http.StatusNotFound, ErrorCodeNotFound)
	found := false
	for _, a := range e.auditRows() {
		found = found || (a.Action == "jobs:cancel" && a.TargetName == id)
	}
	if !found {
		t.Error("the cancel was not audited")
	}
}

func (e *env) auditRows() []store.AuditEntry {
	e.t.Helper()
	rows, err := e.db.AuditEntries(e.t.Context(), 100)
	if err != nil {
		e.t.Fatal(err)
	}
	return rows
}

func TestListJobs(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	for _, app := range []string{"blog-api", "docs-site", "blog-api"} {
		admin.waitJob(admin.startAction(app, "restart"))
	}
	var page JobPage
	admin.do(http.MethodGet, "/jobs?limit=2", nil).decode(t, &page)
	if len(page.Items) != 2 || page.NextCursor == nil || *page.Items[0].App != "blog-api" {
		t.Fatalf("page 1 = %+v", page)
	}
	cursor := *page.NextCursor
	page = JobPage{}
	admin.do(http.MethodGet, "/jobs?limit=2&cursor="+cursor, nil).decode(t, &page)
	if len(page.Items) != 1 || page.NextCursor != nil {
		t.Errorf("page 2 = %+v", page)
	}
	admin.do(http.MethodGet, "/jobs?app=blog-api", nil).decode(t, &page)
	if len(page.Items) != 2 {
		t.Errorf("blog-api jobs = %d", len(page.Items))
	}
	expectError(t, admin.do(http.MethodGet, "/jobs?limit=0", nil), http.StatusBadRequest, ErrorCodeValidationFailed)
}

// LOG-1: tail 100, 500 or 1000 lines, with a process filter. Viewers may read logs.
func TestLOG1Tail(t *testing.T) {
	e := newEnv(t)
	viewer, _ := e.member(e.admin(), "victor", RoleViewer)
	var tail LogTail
	res := viewer.do(http.MethodGet, "/apps/blog-api/logs?lines=500&process=web.2", nil)
	expect(t, res, http.StatusOK)
	res.decode(t, &tail)
	if len(tail.Lines) != 500 || *tail.Lines[0].Process != "web.2" || tail.Lines[0].Ts == nil ||
		strings.Join(tail.Processes, ",") != "web.1,web.2,worker.1" {
		t.Errorf("tail = %d lines, first %+v, processes %v", len(tail.Lines), tail.Lines[0], tail.Processes)
	}
	viewer.do(http.MethodGet, "/apps/blog-api/logs", nil).decode(t, &tail)
	if len(tail.Lines) != 100 {
		t.Errorf("default tail = %d lines", len(tail.Lines))
	}
	expectError(t, viewer.do(http.MethodGet, "/apps/blog-api/logs?lines=42", nil), http.StatusBadRequest, ErrorCodeValidationFailed)
	expectError(t, viewer.do(http.MethodGet, "/apps/blog-api/logs?process=web%3Bid", nil), http.StatusBadRequest, ErrorCodeValidationFailed)
	expectError(t, viewer.do(http.MethodGet, "/apps/nope/logs", nil), http.StatusNotFound, ErrorCodeNotFound)
}

// LOG-2, LOG-4, EVT-3: follow over SSE; at most 5 streams per user.
func TestLOG2FollowAndLimits(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	var streams []<-chan sseEvent
	for range maxStreamsPerUser {
		events, status := admin.openSSE(ctx, "/apps/blog-api/logs/stream?process=worker", nil)
		if status != http.StatusOK {
			t.Fatalf("status %d", status)
		}
		streams = append(streams, events)
	}
	expectError(t, admin.do(http.MethodGet, "/apps/blog-api/logs/stream", nil), http.StatusTooManyRequests, ErrorCodeTooManyStreams)

	timeout := time.After(5 * time.Second)
	var line LogLine
	sawHeartbeat := false
	for line.Text == "" || !sawHeartbeat {
		select {
		case ev := <-streams[0]:
			if ev.name == "line" {
				_ = json.Unmarshal([]byte(ev.data), &line)
			}
			sawHeartbeat = sawHeartbeat || ev.data == "heartbeat"
		case <-timeout:
			t.Fatalf("no line or heartbeat; line %+v", line)
		}
	}
	if line.Process == nil || *line.Process != "worker.1" {
		t.Errorf("line = %+v", line)
	}

	// Closing the streams frees the slots.
	cancel()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		res := admin.do(http.MethodGet, "/apps/blog-api/logs/stream?process=nope.1", nil)
		if res.status == http.StatusOK {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Error("the stream slots were not released")
}

// ACT-2: filters, cursor pages and the CSV export for admins.
func TestACT2AuditListAndExport(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	viewer, _ := e.member(admin, "victor", RoleViewer)
	expect(t, admin.do(http.MethodPost, "/projects", CreateProjectRequest{Name: "=cmd|' /C calc'!A0", Slug: "blog"}), http.StatusCreated)
	admin.waitJob(admin.startAction("status-page", "rebuild"))

	var page AuditPage
	res := viewer.do(http.MethodGet, "/audit?limit=2", nil)
	expect(t, res, http.StatusOK)
	res.decode(t, &page)
	if len(page.Items) != 2 || page.NextCursor == nil || page.Items[0].Action != "apps:rebuild" ||
		page.Items[0].JobId == nil || len(page.Actors) < 2 {
		t.Fatalf("page = %+v", page)
	}
	viewer.do(http.MethodGet, "/audit?result=fail", nil).decode(t, &page)
	if len(page.Items) != 1 || page.Items[0].Result != ActivityEntryResultFail {
		t.Errorf("failures = %+v", page.Items)
	}
	viewer.do(http.MethodGet, "/audit?project=blog", nil).decode(t, &page)
	if len(page.Items) != 1 || page.Items[0].ProjectSlug != "blog" {
		t.Errorf("project = %+v", page.Items)
	}
	page = AuditPage{}
	viewer.do(http.MethodGet, "/audit?user=victor&q=create", nil).decode(t, &page)
	if len(page.Items) != 0 {
		t.Errorf("victor created nothing: %+v", page.Items)
	}
	expectError(t, viewer.do(http.MethodGet, "/audit?result=maybe", nil), http.StatusBadRequest, ErrorCodeValidationFailed)

	expectError(t, viewer.do(http.MethodGet, "/audit/export", nil), http.StatusForbidden, ErrorCodeForbidden)
	res = admin.do(http.MethodGet, "/audit/export?q=projects", nil)
	expect(t, res, http.StatusOK)
	if ct := res.header.Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") ||
		!strings.Contains(res.header.Get("Content-Disposition"), "attachment") {
		t.Errorf("headers = %v", res.header)
	}
	rows, err := csv.NewReader(strings.NewReader(string(res.body))).ReadAll()
	if err != nil || len(rows) != 2 || rows[0][0] != "id" || rows[1][4] != "projects:create" {
		t.Fatalf("csv = %v, %v", rows, err)
	}
	if detail := rows[1][11]; !strings.Contains(detail, `"name":"=cmd`) {
		t.Errorf("detail = %q", detail)
	}
}

func TestCSVSafe(t *testing.T) {
	for in, want := range map[string]string{"=1+1": "'=1+1", "+x": "'+x", "-x": "'-x", "@x": "'@x", "blog": "blog", "": ""} {
		if got := csvSafe(in); got != want {
			t.Errorf("csvSafe(%q) = %q", in, got)
		}
	}
}
