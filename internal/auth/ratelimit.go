package auth

import (
	"strings"
	"sync"
	"time"
)

// LoginLimiter blocks a (username, client IP) pair for Window after MaxFailures failed
// sign-ins within Window (AUTH-3). State is in memory, which the spec allows.
type LoginLimiter struct {
	MaxFailures int
	Window      time.Duration

	mu      sync.Mutex
	entries map[string]*limitEntry
	lastGC  time.Time
}

type limitEntry struct {
	failures     []time.Time
	blockedUntil time.Time
}

// NewLoginLimiter returns the limiter with the spec values: 5 failures in 15 minutes.
func NewLoginLimiter() *LoginLimiter {
	return &LoginLimiter{MaxFailures: 5, Window: 15 * time.Minute, entries: map[string]*limitEntry{}}
}

func limitKey(username, ip string) string { return strings.ToLower(username) + "\x00" + ip }

// Blocked reports whether the pair is blocked at now.
func (l *LoginLimiter) Blocked(username, ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	e := l.entries[limitKey(username, ip)]
	return e != nil && now.Before(e.blockedUntil)
}

// Fail records a failed sign-in.
func (l *LoginLimiter) Fail(username, ip string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.gc(now)
	key := limitKey(username, ip)
	e := l.entries[key]
	if e == nil {
		e = &limitEntry{}
		l.entries[key] = e
	}
	e.failures = append(recent(e.failures, now, l.Window), now)
	if len(e.failures) >= l.MaxFailures {
		e.blockedUntil = now.Add(l.Window)
		e.failures = nil
	}
}

// Succeed clears the failures of the pair.
func (l *LoginLimiter) Succeed(username, ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, limitKey(username, ip))
}

func recent(ts []time.Time, now time.Time, window time.Duration) []time.Time {
	out := ts[:0]
	for _, t := range ts {
		if now.Sub(t) < window {
			out = append(out, t)
		}
	}
	return out
}

// gc drops idle entries at most once per window, so memory stays bounded.
func (l *LoginLimiter) gc(now time.Time) {
	if now.Sub(l.lastGC) < l.Window {
		return
	}
	l.lastGC = now
	for k, e := range l.entries {
		if !now.Before(e.blockedUntil) && len(recent(e.failures, now, l.Window)) == 0 {
			delete(l.entries, k)
		}
	}
}
