package server

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

const testIndex = `<!doctype html><html><head>
<script>document.documentElement.dataset.theme = "dark";</script>
<script type="module" src="/assets/entry-abc123.js"></script>
</head><body></body></html>`

func newTestServer(t *testing.T, files fstest.MapFS) http.Handler {
	t.Helper()
	h, err := New(Options{
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		Version: "test",
		Static:  files,
	})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func defaultFiles() fstest.MapFS {
	return fstest.MapFS{
		"index.html":             {Data: []byte(testIndex)},
		"assets/entry-abc123.js": {Data: []byte("console.log(1)")},
		"favicon.svg":            {Data: []byte("<svg/>")},
	}
}

func get(t *testing.T, h http.Handler, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec
}

func TestHealthz(t *testing.T) {
	rec := get(t, newTestServer(t, defaultFiles()), http.MethodGet, "/healthz")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" || body["version"] != "test" {
		t.Errorf("body = %v", body)
	}
}

func TestSecurityHeaders(t *testing.T) {
	h := newTestServer(t, defaultFiles())
	for _, target := range []string{"/healthz", "/", "/api/v1/nope", "/assets/entry-abc123.js"} {
		rec := get(t, h, http.MethodGet, target)
		for header, want := range map[string]string{
			"X-Content-Type-Options": "nosniff",
			"Referrer-Policy":        "same-origin",
			"X-Frame-Options":        "DENY",
		} {
			if got := rec.Header().Get(header); got != want {
				t.Errorf("%s: %s = %q, want %q", target, header, got, want)
			}
		}
		if !strings.Contains(rec.Header().Get("Content-Security-Policy"), "default-src 'self'") {
			t.Errorf("%s: missing CSP", target)
		}
	}
}

func TestCSPHashesInlineScripts(t *testing.T) {
	csp := get(t, newTestServer(t, defaultFiles()), http.MethodGet, "/").Header().Get("Content-Security-Policy")
	sum := sha256.Sum256([]byte(`document.documentElement.dataset.theme = "dark";`))
	want := "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
	if !strings.Contains(csp, "script-src 'self' "+want) {
		t.Errorf("CSP %q does not allow the inline script %s", csp, want)
	}
	if strings.Contains(csp, "unsafe-inline") {
		t.Errorf("CSP must not use unsafe-inline: %q", csp)
	}
	if strings.Count(csp, "sha256-") != 1 {
		t.Errorf("only the inline script should be hashed: %q", csp)
	}
}

func TestSPAFallback(t *testing.T) {
	h := newTestServer(t, defaultFiles())
	for _, target := range []string{"/", "/index.html", "/apps/web", "/projects/shop/config"} {
		rec := get(t, h, http.MethodGet, target)
		if rec.Code != http.StatusOK || rec.Body.String() != testIndex {
			t.Errorf("%s: status %d, body %q", target, rec.Code, rec.Body.String())
		}
		if rec.Header().Get("Cache-Control") != "no-cache" {
			t.Errorf("%s: Cache-Control = %q", target, rec.Header().Get("Cache-Control"))
		}
	}
}

func TestStaticFiles(t *testing.T) {
	h := newTestServer(t, defaultFiles())

	rec := get(t, h, http.MethodGet, "/assets/entry-abc123.js")
	if rec.Code != http.StatusOK || rec.Body.String() != "console.log(1)" {
		t.Fatalf("asset: status %d, body %q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Header().Get("Cache-Control"), "immutable") {
		t.Errorf("hashed asset should be immutable, got %q", rec.Header().Get("Cache-Control"))
	}

	if rec := get(t, h, http.MethodGet, "/favicon.svg"); rec.Header().Get("Cache-Control") != "no-cache" {
		t.Errorf("unhashed file Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
	if rec := get(t, h, http.MethodGet, "/assets/missing.js"); rec.Code != http.StatusNotFound {
		t.Errorf("missing asset: status %d", rec.Code)
	}
	if rec := get(t, h, http.MethodPost, "/apps/web"); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST to SPA route: status %d", rec.Code)
	}
}

func TestUnknownAPIRouteIsJSON(t *testing.T) {
	rec := get(t, newTestServer(t, defaultFiles()), http.MethodGet, "/api/v1/does-not-exist")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not the error shape: %v", err)
	}
	if body.Error.Code != "not_found" {
		t.Errorf("code = %q", body.Error.Code)
	}
}

func TestNotBuiltNotice(t *testing.T) {
	rec := get(t, newTestServer(t, fstest.MapFS{}), http.MethodGet, "/")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "task build") {
		t.Errorf("status %d, body %q", rec.Code, rec.Body.String())
	}
}

func TestRecoverer(t *testing.T) {
	h := recoverer(slog.New(slog.NewTextHandler(io.Discard, nil)))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))
	rec := get(t, h, http.MethodGet, "/")
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), `"internal"`) {
		t.Errorf("status %d, body %q", rec.Code, rec.Body.String())
	}
}
