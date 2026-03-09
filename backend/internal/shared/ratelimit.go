package shared

import (
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// FixedWindowRateLimiterConfig defines per-key fixed-window rate limiting settings.
type FixedWindowRateLimiterConfig struct {
	Name        string
	MaxRequests int
	Window      time.Duration
	KeyFunc     func(*http.Request) string
}

type fixedWindowBucket struct {
	count   int
	resetAt time.Time
}

// TokenBucketRateLimiterConfig defines token-bucket rate limiting settings.
type TokenBucketRateLimiterConfig struct {
	Name              string
	RequestsPerMinute int
	Burst             int
	KeyFunc           func(*http.Request) string
}

type tokenBucketState struct {
	tokens     float64
	lastRefill time.Time
}

// FailedAttemptLockoutConfig defines failed-attempt lockout settings.
type FailedAttemptLockoutConfig struct {
	Name        string
	MaxFailures int
	Window      time.Duration
	Lockout     time.Duration
}

type failedAttemptState struct {
	failures     []time.Time
	lockoutUntil time.Time
	lastSeen     time.Time
}

// FixedWindowRateLimiter enforces a simple fixed-window request cap by key.
type FixedWindowRateLimiter struct {
	name        string
	maxRequests int
	window      time.Duration
	keyFn       func(*http.Request) string

	mu      sync.Mutex
	buckets map[string]fixedWindowBucket
}

// TokenBucketRateLimiter enforces burst-aware limits per key.
type TokenBucketRateLimiter struct {
	name         string
	refillPerSec float64
	capacity     float64
	keyFn        func(*http.Request) string

	mu      sync.Mutex
	buckets map[string]tokenBucketState
}

// FailedAttemptLockoutLimiter tracks failed attempts and temporary lockouts.
type FailedAttemptLockoutLimiter struct {
	name        string
	maxFailures int
	window      time.Duration
	lockout     time.Duration

	mu     sync.Mutex
	states map[string]failedAttemptState
}

// NewFixedWindowRateLimiter creates a limiter with sane defaults.
func NewFixedWindowRateLimiter(cfg FixedWindowRateLimiterConfig) *FixedWindowRateLimiter {
	maxRequests := cfg.MaxRequests
	if maxRequests <= 0 {
		maxRequests = 1
	}
	window := cfg.Window
	if window <= 0 {
		window = time.Minute
	}
	keyFn := cfg.KeyFunc
	if keyFn == nil {
		keyFn = ClientIPRateLimitKey
	}

	return &FixedWindowRateLimiter{
		name:        cfg.Name,
		maxRequests: maxRequests,
		window:      window,
		keyFn:       keyFn,
		buckets:     make(map[string]fixedWindowBucket),
	}
}

// NewTokenBucketRateLimiter creates a burst-aware limiter with sane defaults.
func NewTokenBucketRateLimiter(cfg TokenBucketRateLimiterConfig) *TokenBucketRateLimiter {
	requestsPerMinute := cfg.RequestsPerMinute
	if requestsPerMinute <= 0 {
		requestsPerMinute = 60
	}
	burst := cfg.Burst
	if burst <= 0 {
		burst = 1
	}
	keyFn := cfg.KeyFunc
	if keyFn == nil {
		keyFn = ClientIPRateLimitKey
	}

	return &TokenBucketRateLimiter{
		name:         cfg.Name,
		refillPerSec: float64(requestsPerMinute) / 60.0,
		capacity:     float64(burst),
		keyFn:        keyFn,
		buckets:      make(map[string]tokenBucketState),
	}
}

// NewFailedAttemptLockoutLimiter creates a failed-attempt lockout limiter.
func NewFailedAttemptLockoutLimiter(cfg FailedAttemptLockoutConfig) *FailedAttemptLockoutLimiter {
	maxFailures := cfg.MaxFailures
	if maxFailures <= 0 {
		maxFailures = 5
	}
	window := cfg.Window
	if window <= 0 {
		window = 10 * time.Minute
	}
	lockout := cfg.Lockout
	if lockout <= 0 {
		lockout = 15 * time.Minute
	}
	return &FailedAttemptLockoutLimiter{
		name:        cfg.Name,
		maxFailures: maxFailures,
		window:      window,
		lockout:     lockout,
		states:      make(map[string]failedAttemptState),
	}
}

// Wrap applies per-key fixed-window limiting and returns 429 when exceeded.
func (l *FixedWindowRateLimiter) Wrap(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		now := time.Now().UTC()
		key := strings.TrimSpace(l.keyFn(r))
		if key == "" {
			key = "unknown"
		}

		allow, retryAfterS := l.allow(now, key)
		if !allow {
			w.Header().Set("Retry-After", strconv.Itoa(retryAfterS))
			WriteError(w, ErrRateLimited, "Too many requests", "RATE_LIMITED")
			return
		}

		next(w, r)
	}
}

// Wrap applies per-key token-bucket limiting and returns 429 when exceeded.
func (l *TokenBucketRateLimiter) Wrap(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		now := time.Now().UTC()
		key := strings.TrimSpace(l.keyFn(r))
		if key == "" {
			key = "unknown"
		}

		allow, retryAfterS := l.allow(now, key)
		if !allow {
			w.Header().Set("Retry-After", strconv.Itoa(retryAfterS))
			WriteError(w, ErrRateLimited, "Too many requests", "RATE_LIMITED")
			return
		}
		next(w, r)
	}
}

// Allow returns whether the key is currently allowed and retry-after seconds when blocked.
func (l *FailedAttemptLockoutLimiter) Allow(key string, now time.Time) (bool, int) {
	key = strings.TrimSpace(key)
	if key == "" {
		return true, 0
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	s := l.states[key]
	s.lastSeen = now
	if !s.lockoutUntil.IsZero() && now.Before(s.lockoutUntil) {
		l.states[key] = s
		return false, retryAfterSeconds(now, s.lockoutUntil)
	}

	if !s.lockoutUntil.IsZero() && !now.Before(s.lockoutUntil) {
		s.lockoutUntil = time.Time{}
	}
	s.failures = pruneFailures(s.failures, now, l.window)
	if len(s.failures) == 0 && s.lockoutUntil.IsZero() {
		delete(l.states, key)
		return true, 0
	}

	l.states[key] = s
	return true, 0
}

// RegisterFailure records a failed attempt and returns lockout state.
func (l *FailedAttemptLockoutLimiter) RegisterFailure(key string, now time.Time) (bool, int) {
	key = strings.TrimSpace(key)
	if key == "" {
		return false, 0
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	s := l.states[key]
	s.lastSeen = now
	if !s.lockoutUntil.IsZero() && now.Before(s.lockoutUntil) {
		l.states[key] = s
		return true, retryAfterSeconds(now, s.lockoutUntil)
	}
	if !s.lockoutUntil.IsZero() && !now.Before(s.lockoutUntil) {
		s.lockoutUntil = time.Time{}
		s.failures = s.failures[:0]
	}

	s.failures = pruneFailures(s.failures, now, l.window)
	s.failures = append(s.failures, now)
	if len(s.failures) >= l.maxFailures {
		s.lockoutUntil = now.Add(l.lockout)
		s.failures = nil
		l.states[key] = s
		return true, retryAfterSeconds(now, s.lockoutUntil)
	}

	l.states[key] = s
	l.evictStaleLocked(now)
	return false, 0
}

// Reset clears all failure/lockout state for a key, usually on successful auth.
func (l *FailedAttemptLockoutLimiter) Reset(key string) {
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	l.mu.Lock()
	delete(l.states, key)
	l.mu.Unlock()
}

func (l *FixedWindowRateLimiter) allow(now time.Time, key string) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	b := l.buckets[key]
	if b.resetAt.IsZero() || !now.Before(b.resetAt) {
		b = fixedWindowBucket{
			count:   0,
			resetAt: now.Add(l.window),
		}
	}

	if b.count >= l.maxRequests {
		retryAfter := int(time.Until(b.resetAt).Seconds())
		if retryAfter < 1 {
			retryAfter = 1
		}
		l.buckets[key] = b
		return false, retryAfter
	}

	b.count++
	l.buckets[key] = b
	return true, 0
}

func (l *TokenBucketRateLimiter) allow(now time.Time, key string) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	b := l.buckets[key]
	if b.lastRefill.IsZero() {
		b = tokenBucketState{
			tokens:     l.capacity,
			lastRefill: now,
		}
	}

	if now.After(b.lastRefill) {
		elapsed := now.Sub(b.lastRefill).Seconds()
		b.tokens += elapsed * l.refillPerSec
		if b.tokens > l.capacity {
			b.tokens = l.capacity
		}
		b.lastRefill = now
	}

	if b.tokens < 1.0 {
		l.buckets[key] = b
		missing := 1.0 - b.tokens
		retryAfter := int(math.Ceil(missing / l.refillPerSec))
		if retryAfter < 1 {
			retryAfter = 1
		}
		return false, retryAfter
	}

	b.tokens -= 1.0
	l.buckets[key] = b
	return true, 0
}

// ClientIPRateLimitKey identifies a client by best-effort IP extraction.
func ClientIPRateLimitKey(r *http.Request) string {
	return ClientIPFromRequest(r)
}

// SessionCodeRateLimitKey identifies a client by authenticated session code.
func SessionCodeRateLimitKey(r *http.Request) string {
	if r == nil {
		return ""
	}
	claims, ok := SessionAuthClaimsFromContext(r.Context())
	if !ok {
		return ""
	}
	return strings.TrimSpace(claims.SessionCode)
}

// ClientIPFromRequest extracts client IP from common proxy headers or RemoteAddr.
func ClientIPFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}

	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			first := strings.TrimSpace(parts[0])
			if first != "" {
				return first
			}
		}
	}

	if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" {
		return xri
	}

	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func pruneFailures(times []time.Time, now time.Time, window time.Duration) []time.Time {
	if len(times) == 0 {
		return times
	}
	cutoff := now.Add(-window)
	kept := times[:0]
	for _, ts := range times {
		if !ts.Before(cutoff) {
			kept = append(kept, ts)
		}
	}
	return kept
}

func retryAfterSeconds(now, until time.Time) int {
	retry := int(time.Until(until).Seconds())
	if now.Before(until) {
		retry = int(until.Sub(now).Seconds())
	}
	if retry < 1 {
		retry = 1
	}
	return retry
}

func (l *FailedAttemptLockoutLimiter) evictStaleLocked(now time.Time) {
	if len(l.states) < 4096 {
		return
	}

	cutoff := now.Add(-(l.window + l.lockout))
	for key, s := range l.states {
		if s.lastSeen.Before(cutoff) && s.lockoutUntil.IsZero() && len(s.failures) == 0 {
			delete(l.states, key)
		}
	}
}
