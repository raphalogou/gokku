package api

import (
	"context"
	"net"
	"net/http"

	"gokku/internal/auth"
)

// RequestInfo is what the server learned about the client: its address, after trusted
// proxies (SEC-3a), and whether the request reached the proxy or Gokku over HTTPS.
type RequestInfo struct {
	ClientIP string
	HTTPS    bool
}

type requestInfoKey struct{}

// WithRequestInfo returns a context carrying info. The server middleware sets it.
func WithRequestInfo(ctx context.Context, info RequestInfo) context.Context {
	return context.WithValue(ctx, requestInfoKey{}, info)
}

func requestInfo(r *http.Request) RequestInfo {
	info, _ := r.Context().Value(requestInfoKey{}).(RequestInfo)
	return info
}

func clientInfo(r *http.Request) auth.ClientInfo {
	return auth.ClientInfo{IP: requestInfo(r).ClientIP, UserAgent: r.UserAgent()}
}

// baseURL is the origin the browser used, for links such as invites.
func baseURL(r *http.Request) string {
	scheme := "http"
	if requestInfo(r).HTTPS {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// requestHost is the host name the browser used, without a port, for git remote
// instructions when GOKKU_DOKKU_HOST is not set.
func requestHost(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.Host); err == nil {
		return host
	}
	return r.Host
}
