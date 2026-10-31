package api

import (
	"net/http"
	"strings"
	"testing"
)

// SCL-1: read the formation, change it as a job, refuse unknown types.
func TestSCL1Scale(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	viewer, _ := e.member(admin, "victor", RoleViewer)
	operator, _ := e.member(admin, "olga", RoleOperator)

	var s AppScale
	res := viewer.do(http.MethodGet, "/apps/blog-api/scale", nil)
	expect(t, res, http.StatusOK)
	res.decode(t, &s)
	if !s.CanScale || len(s.Processes) != 2 || s.Processes[0] != (ProcessScale{Type: "web", Quantity: 2, Running: 2}) {
		t.Errorf("scale = %+v", s)
	}
	body := ScaleRequest{Processes: map[string]int{"worker": 2}}
	expectError(t, viewer.do(http.MethodPut, "/apps/blog-api/scale", body), http.StatusForbidden, ErrorCodeForbidden)
	var ref JobRef
	res = operator.do(http.MethodPut, "/apps/blog-api/scale", body)
	expect(t, res, http.StatusAccepted)
	res.decode(t, &ref)
	if j := operator.waitJob(ref.JobId); j.Status != JobStatusSucceeded || j.Kind != "app.scale" {
		t.Errorf("job = %+v", j)
	}
	if a := e.lastAudit(); a.Action != "apps:scale" || !strings.Contains(a.Detail, `"worker":{"from":1,"to":2}`) {
		t.Errorf("audit = %+v", a)
	}
	_ = e.snap.Refresh(t.Context())
	viewer.do(http.MethodGet, "/apps/blog-api/scale", nil).decode(t, &s)
	if s.Processes[1] != (ProcessScale{Type: "worker", Quantity: 2, Running: 2}) {
		t.Errorf("after = %+v", s)
	}

	for _, bad := range []map[string]int{{"cron": 1}, {"web": 101}, {"web": 2}} {
		expectError(t, operator.do(http.MethodPut, "/apps/blog-api/scale", ScaleRequest{Processes: bad}),
			http.StatusBadRequest, ErrorCodeValidationFailed)
	}
}
