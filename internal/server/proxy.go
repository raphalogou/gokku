package server

import (
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"

	"gokku/internal/api"
)

// proxyAware resolves the client address and scheme, honouring X-Forwarded-For and
// X-Forwarded-Proto only from trusted proxies (SEC-3a).
type proxyAware struct {
	trusted []netip.Prefix
	// warnUntrusted is set when Gokku listens on loopback with no trusted proxies: a request
	// with X-Forwarded-For then means a proxy is in front but GOKKU_TRUSTED_PROXIES is unset.
	warnUntrusted bool
	warnOnce      sync.Once
	log           *slog.Logger
}

func newProxyAware(trusted []netip.Prefix, listen string, log *slog.Logger) *proxyAware {
	return &proxyAware{trusted: trusted, warnUntrusted: len(trusted) == 0 && isLoopback(listen), log: log}
}

func (p *proxyAware) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info := p.resolve(r)
		next.ServeHTTP(w, r.WithContext(api.WithRequestInfo(r.Context(), info)))
	})
}

func (p *proxyAware) resolve(r *http.Request) api.RequestInfo {
	remote := remoteAddr(r)
	info := api.RequestInfo{ClientIP: remote.String(), HTTPS: r.TLS != nil}
	if !remote.IsValid() {
		info.ClientIP = r.RemoteAddr
	}
	if !p.isTrusted(remote) {
		if p.warnUntrusted && r.Header.Get("X-Forwarded-For") != "" {
			p.warnOnce.Do(func() {
				p.log.Warn("request has X-Forwarded-For but GOKKU_TRUSTED_PROXIES is empty: " +
					"client addresses and HTTPS are not detected, so rate limiting sees the proxy and the session cookie " +
					"is not marked Secure. For the documented nginx setup, set GOKKU_TRUSTED_PROXIES=127.0.0.1/32.")
			})
		}
		return info
	}

	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		info.HTTPS = strings.EqualFold(strings.TrimSpace(lastValue(proto)), "https")
	}
	// Walk X-Forwarded-For from the right: the first address that is not a trusted proxy is
	// the client. Addresses further left were supplied by the client and cannot be trusted.
	hops := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		addr, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			break
		}
		addr = addr.Unmap()
		info.ClientIP = addr.String()
		if !p.isTrusted(addr) {
			break
		}
	}
	return info
}

func (p *proxyAware) isTrusted(addr netip.Addr) bool {
	if !addr.IsValid() {
		return false
	}
	for _, pref := range p.trusted {
		if pref.Contains(addr) {
			return true
		}
	}
	return false
}

func remoteAddr(r *http.Request) netip.Addr {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}
	}
	return addr.Unmap()
}

func lastValue(header string) string {
	if i := strings.LastIndexByte(header, ','); i >= 0 {
		return header[i+1:]
	}
	return header
}

func isLoopback(listen string) bool {
	host, _, err := net.SplitHostPort(listen)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	addr, err := netip.ParseAddr(host)
	return err == nil && addr.IsLoopback()
}
