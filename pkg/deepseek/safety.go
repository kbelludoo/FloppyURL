// Package deepseek implements a free, browser-session based client for
// chat.deepseek.com that needs no API key.
//
// It mirrors the flow of the reference Python implementation:
//
//  1. authenticate by capturing the signed-in web session (token in
//     localStorage + cookies) from a real Chrome instance over CDP;
//  2. obtain a proof-of-work challenge and solve it with DeepSeek's own
//     WebAssembly module (DeepSeekHashV1);
//  3. POST the completion request and parse the SSE stream into text.
//
// The account-safety controls here intentionally fail closed. They do not
// bypass provider limits, CAPTCHAs, WAFs or account controls.
package deepseek

import (
	"strings"
	"sync"
	"time"
)

// SafetyTrip is raised when the client must stop instead of retrying
// automatically (CAPTCHA / WAF / account-protection signal).
type SafetyTrip struct{ Reason string }

func (e *SafetyTrip) Error() string {
	if e.Reason == "" {
		return "DeepSeek safety guard is active."
	}
	return e.Reason
}

// RequestGuard paces requests and trips a local circuit breaker on
// account-protection responses.
type RequestGuard struct {
	MinInterval time.Duration

	mu          sync.Mutex
	lastRequest time.Time
	tripped     bool
	reason      string
}

// NewRequestGuard builds a guard with a minimum spacing between requests.
func NewRequestGuard(minInterval time.Duration) *RequestGuard {
	if minInterval < time.Second {
		minInterval = time.Second
	}
	return &RequestGuard{MinInterval: minInterval}
}

// Wait blocks until the minimum interval has elapsed since the previous
// request, or returns a SafetyTrip error if the guard has tripped.
func (g *RequestGuard) Wait() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.tripped {
		if g.reason == "" {
			return &SafetyTrip{}
		}
		return &SafetyTrip{Reason: g.reason}
	}
	now := time.Now()
	if delay := g.MinInterval - now.Sub(g.lastRequest); delay > 0 {
		time.Sleep(delay)
	}
	g.lastRequest = time.Now()
	return nil
}

// Trip activates the circuit breaker so no further requests are made.
func (g *RequestGuard) Trip(reason string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.tripped = true
	g.reason = reason
}

// Tripped reports whether the guard is currently open.
func (g *RequestGuard) Tripped() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.tripped
}

// Reason returns the reason the guard tripped, if any.
func (g *RequestGuard) Reason() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.reason
}

// Reset clears the circuit breaker.
func (g *RequestGuard) Reset() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.tripped = false
	g.reason = ""
	g.lastRequest = time.Now()
}

var protectionMarkers = []string{
	"captcha", "recaptcha", "verify you are human", "human verification",
	"too many requests", "rate limit", "forbidden", "account suspended",
	"account disabled", "risk control", "security verification",
}

// IsAccountProtectionResponse reports whether an HTTP response looks like an
// anti-abuse / account-protection signal that must stop automation.
func IsAccountProtectionResponse(status int, body string) bool {
	if status == 401 || status == 403 || status == 429 {
		return true
	}
	text := strings.ToLower(body)
	for _, m := range protectionMarkers {
		if strings.Contains(text, m) {
			return true
		}
	}
	return false
}

var authMarkers = []string{
	"authorization failed", "invalid token", "unauthorized",
	"token expired", "token invalid", "login required",
	"please login", "please log in", "not logged in",
}

// isAuthFailure reports whether the server is rejecting our credentials
// (rather than rate/WAF). HTTP 401 is near-always auth; 403/429 are left to
// the protection path unless the body carries an explicit auth marker.
func isAuthFailure(status int, body string) bool {
	if status == 401 {
		return true
	}
	text := strings.ToLower(body)
	for _, m := range authMarkers {
		if strings.Contains(text, m) {
			return true
		}
	}
	return false
}
