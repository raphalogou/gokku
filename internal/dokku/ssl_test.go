package dokku

import (
	"errors"
	"slices"
	"testing"
	"time"
)

func TestParseLetsencryptFixture(t *testing.T) {
	r, err := parseLetsencrypt(fixture(t, "letsencrypt-report.txt"), "blog-api")
	if err != nil {
		t.Fatal(err)
	}
	if !r.Active || !r.AutoRenew || r.Expires.Unix() != 1798365600 || r.Email != "" || r.ComputedEmail != "ops@example.com" {
		t.Errorf("report = %+v", r)
	}
	if _, err := parseLetsencrypt(fixture(t, "letsencrypt-report.txt"), "other"); err == nil {
		t.Error("a report for another app must be an error")
	}
}

func TestValidDomain(t *testing.T) {
	for d, ok := range map[string]bool{
		"example.com": true, "blog-api.example.com": true, "*.example.com": true, "localhost": true, "a1.b2": true,
		"": false, "-x.com": false, "x-.com": false, "Example.com": false, "a..b": false, "a b.com": false,
		"x.com/path": false, "*.*.x.com": false, "--flag": false,
	} {
		if ValidDomain(d) != ok {
			t.Errorf("ValidDomain(%q) = %v", d, !ok)
		}
	}
}

// DOM-1, SSL-1 on the fake runner.
func TestDomainsAndLetsencryptOnFake(t *testing.T) {
	fake := NewFakeRunner(time.Now())
	cl := NewClient(fake, discard())
	ctx := t.Context()
	if err := cl.AddDomain(ctx, "docs-site", "help.example.com"); err != nil {
		t.Fatal(err)
	}
	reports, _ := cl.AppReports(ctx)
	if !slices.Contains(reports["docs-site"].Domains, "help.example.com") {
		t.Errorf("domains = %v", reports["docs-site"].Domains)
	}
	if err := cl.RemoveDomain(ctx, "docs-site", "help.example.com"); err != nil {
		t.Fatal(err)
	}
	if err := cl.AddDomain(ctx, "docs-site", "--global"); !errors.Is(err, ErrInvalidName) {
		t.Errorf("a flag was accepted as a domain: %v", err)
	}

	r, err := cl.Letsencrypt(ctx, "docs-site")
	if err != nil || r.Active || !r.AutoRenew || r.ComputedEmail != "ops@example.com" {
		t.Fatalf("before = %+v, %v", r, err)
	}
	if err := cl.SetLetsencryptEmail(ctx, "docs-site", "docs@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := cl.SetLetsencryptEmail(ctx, "docs-site", "not an email"); !errors.Is(err, ErrInvalidName) {
		t.Errorf("bad email: %v", err)
	}
	var lines int
	if err := cl.EnableLetsencrypt(ctx, "docs-site", false, func(Line) { lines++ }); err != nil || lines == 0 {
		t.Fatalf("enable: %v (%d lines)", err, lines)
	}
	r, _ = cl.Letsencrypt(ctx, "docs-site")
	if !r.Active || r.Email != "docs@example.com" || time.Until(r.Expires) < 89*24*time.Hour-2*time.Hour {
		t.Errorf("after enable = %+v", r)
	}
	if err := cl.SetLetsencryptAutoRenew(ctx, false); err != nil {
		t.Fatal(err)
	}
	if err := cl.DisableLetsencrypt(ctx, "docs-site", func(Line) {}); err != nil {
		t.Fatal(err)
	}
	r, _ = cl.Letsencrypt(ctx, "docs-site")
	if r.Active || r.AutoRenew || !r.Expires.IsZero() {
		t.Errorf("after disable = %+v", r)
	}
	// An app without domains cannot get a certificate.
	if err := cl.EnableLetsencrypt(ctx, "worker-queue", false, func(Line) {}); err == nil {
		t.Error("enable without domains succeeded")
	}
}
