package server

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

func resolveWith(t *testing.T, trusted []string, remote string, headers map[string]string) (string, bool) {
	t.Helper()
	var prefixes []netip.Prefix
	for _, s := range trusted {
		prefixes = append(prefixes, netip.MustParsePrefix(s))
	}
	p := newProxyAware(prefixes, "127.0.0.1:8080", slog.New(slog.DiscardHandler))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = remote
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	info := p.resolve(r)
	return info.ClientIP, info.HTTPS
}

func TestForwardedHeadersOnlyFromTrustedProxies(t *testing.T) { // SEC-3a
	fwd := map[string]string{"X-Forwarded-For": "203.0.113.9", "X-Forwarded-Proto": "https"}

	ip, https := resolveWith(t, nil, "127.0.0.1:5000", fwd)
	if ip != "127.0.0.1" || https {
		t.Errorf("untrusted proxy honoured: %s %v", ip, https)
	}
	ip, https = resolveWith(t, []string{"127.0.0.1/32"}, "127.0.0.1:5000", fwd)
	if ip != "203.0.113.9" || !https {
		t.Errorf("trusted proxy ignored: %s %v", ip, https)
	}
	ip, _ = resolveWith(t, []string{"127.0.0.1/32"}, "198.51.100.4:5000", fwd)
	if ip != "198.51.100.4" {
		t.Errorf("headers from an untrusted peer honoured: %s", ip)
	}
}

func TestForwardedForIsReadFromTheRight(t *testing.T) {
	// The client can put anything at the left of the header; only the hop the trusted
	// proxy appended is reliable.
	ip, _ := resolveWith(t, []string{"127.0.0.1/32", "10.0.0.0/8"}, "127.0.0.1:1",
		map[string]string{"X-Forwarded-For": "6.6.6.6, 203.0.113.9, 10.1.2.3"})
	if ip != "203.0.113.9" {
		t.Errorf("client IP = %s, want 203.0.113.9", ip)
	}
	ip, _ = resolveWith(t, []string{"127.0.0.1/32"}, "127.0.0.1:1", map[string]string{"X-Forwarded-For": "garbage"})
	if ip != "127.0.0.1" {
		t.Errorf("unparsable header: client IP = %s", ip)
	}
}

func TestUntrustedForwardedWarnsOnce(t *testing.T) { // SEC-3a
	var buf bytes.Buffer
	p := newProxyAware(nil, "127.0.0.1:8080", slog.New(slog.NewTextHandler(&buf, nil)))
	for range 3 {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = "127.0.0.1:1"
		r.Header.Set("X-Forwarded-For", "203.0.113.9")
		p.resolve(r)
	}
	if n := strings.Count(buf.String(), "level=WARN"); n != 1 {
		t.Errorf("warning logged %d times:\n%s", n, buf.String())
	}

	buf.Reset()
	public := newProxyAware(nil, "0.0.0.0:8080", slog.New(slog.NewTextHandler(&buf, nil)))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Forwarded-For", "203.0.113.9")
	public.resolve(r)
	if buf.Len() != 0 {
		t.Errorf("warning on a non-loopback listener: %s", buf.String())
	}
}
