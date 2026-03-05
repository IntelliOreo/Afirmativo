package shared

import (
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

// FixedWindowRateLimiter enforces a simple fixed-window request cap by key.
type FixedWindowRateLimiter struct {
	name        string
	maxRequests int
	window      time.Duration
	keyFn       func(*http.Request) string

	mu      sync.Mutex
	buckets map[string]fixedWindowBucket
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

// ClientIPRateLimitKey identifies a client by best-effort IP extraction.
func ClientIPRateLimitKey(r *http.Request) string {
	return ClientIPFromRequest(r)
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
