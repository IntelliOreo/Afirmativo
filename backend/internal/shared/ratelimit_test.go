package shared

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTokenBucketRateLimiter_AllowsBurstThenRefills(t *testing.T) {
	t.Parallel()

	limiter := NewTokenBucketRateLimiter(TokenBucketRateLimiterConfig{
		Name:              "test_token_bucket",
		RequestsPerMinute: 60, // 1 token/second
		Burst:             2,
	})

	now := time.Unix(1_700_000_000, 0).UTC()
	if ok, _ := limiter.allow(now, "k"); !ok {
		t.Fatalf("first request should be allowed")
	}
	if ok, _ := limiter.allow(now, "k"); !ok {
		t.Fatalf("second request should be allowed")
	}

	if ok, retry := limiter.allow(now, "k"); ok {
		t.Fatalf("third request should be blocked")
	} else if retry < 1 {
		t.Fatalf("retryAfter = %d, want >= 1", retry)
	}

	if ok, _ := limiter.allow(now.Add(1*time.Second), "k"); !ok {
		t.Fatalf("request after refill should be allowed")
	}
}

func TestFailedAttemptLockoutLimiter_LocksAndExpires(t *testing.T) {
	t.Parallel()

	limiter := NewFailedAttemptLockoutLimiter(FailedAttemptLockoutConfig{
		Name:        "test_failed_lockout",
		MaxFailures: 2,
		Window:      10 * time.Minute,
		Lockout:     15 * time.Minute,
	})

	key := "AP-AAAA-BBBB|203.0.113.1"
	now := time.Unix(1_700_000_000, 0).UTC()

	if allowed, _ := limiter.Allow(key, now); !allowed {
		t.Fatalf("expected allow before failures")
	}

	if locked, _ := limiter.RegisterFailure(key, now); locked {
		t.Fatalf("first failure should not lock")
	}

	locked, retry := limiter.RegisterFailure(key, now.Add(10*time.Second))
	if !locked {
		t.Fatalf("second failure should lock")
	}
	if retry < 1 {
		t.Fatalf("retryAfter = %d, want >= 1", retry)
	}

	if allowed, _ := limiter.Allow(key, now.Add(20*time.Second)); allowed {
		t.Fatalf("expected blocked during lockout")
	}

	if allowed, _ := limiter.Allow(key, now.Add(16*time.Minute)); !allowed {
		t.Fatalf("expected allow after lockout expires")
	}
}

func TestFailedAttemptLockoutLimiter_ResetClearsState(t *testing.T) {
	t.Parallel()

	limiter := NewFailedAttemptLockoutLimiter(FailedAttemptLockoutConfig{
		Name:        "test_failed_reset",
		MaxFailures: 2,
		Window:      10 * time.Minute,
		Lockout:     15 * time.Minute,
	})

	key := "AP-AAAA-BBBB|203.0.113.2"
	now := time.Unix(1_700_000_000, 0).UTC()
	_, _ = limiter.RegisterFailure(key, now)
	_, _ = limiter.RegisterFailure(key, now.Add(1*time.Second))

	limiter.Reset(key)

	if allowed, _ := limiter.Allow(key, now.Add(2*time.Second)); !allowed {
		t.Fatalf("expected allow after reset")
	}
}

func TestLayeredTokenBucketLimiters_ApplyPerSessionAndPerIP(t *testing.T) {
	t.Parallel()

	ipLimiter := NewTokenBucketRateLimiter(TokenBucketRateLimiterConfig{
		Name:              "test_voice_ip",
		RequestsPerMinute: 600,
		Burst:             10,
		KeyFunc:           ClientIPRateLimitKey,
	})
	sessionLimiter := NewTokenBucketRateLimiter(TokenBucketRateLimiterConfig{
		Name:              "test_voice_session",
		RequestsPerMinute: 600,
		Burst:             1,
		KeyFunc:           SessionCodeRateLimitKey,
	})

	hits := 0
	wrapped := ipLimiter.Wrap(sessionLimiter.Wrap(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}))

	req1 := httptest.NewRequest(http.MethodPost, "/api/voice/token", nil)
	req1.RemoteAddr = "203.0.113.5:1234"
	req1 = req1.WithContext(WithSessionAuthClaims(req1.Context(), &SessionAuthClaims{SessionCode: "AP-AAAA-BBBB"}))
	rr1 := httptest.NewRecorder()
	wrapped(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("first status = %d, want 200", rr1.Code)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/api/voice/token", nil)
	req2.RemoteAddr = "203.0.113.5:1234"
	req2 = req2.WithContext(WithSessionAuthClaims(req2.Context(), &SessionAuthClaims{SessionCode: "AP-AAAA-BBBB"}))
	rr2 := httptest.NewRecorder()
	wrapped(rr2, req2)
	if rr2.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want 429 (session bucket)", rr2.Code)
	}
	if retry := rr2.Header().Get("Retry-After"); retry == "" {
		t.Fatalf("expected Retry-After header on session limiter block")
	}
	var secondErr ErrorResponse
	if err := json.Unmarshal(rr2.Body.Bytes(), &secondErr); err != nil {
		t.Fatalf("unmarshal second error body: %v", err)
	}
	if secondErr.Code != "RATE_LIMITED" {
		t.Fatalf("second error code = %q, want RATE_LIMITED", secondErr.Code)
	}

	req3 := httptest.NewRequest(http.MethodPost, "/api/voice/token", nil)
	req3.RemoteAddr = "203.0.113.5:1234"
	req3 = req3.WithContext(WithSessionAuthClaims(req3.Context(), &SessionAuthClaims{SessionCode: "AP-CCCC-DDDD"}))
	rr3 := httptest.NewRecorder()
	wrapped(rr3, req3)
	if rr3.Code != http.StatusOK {
		t.Fatalf("third status = %d, want 200 for different session bucket", rr3.Code)
	}

	if hits != 2 {
		t.Fatalf("handler hits = %d, want 2 successful requests", hits)
	}
}

func TestLayeredTokenBucketLimiters_ApplyGlobalIPLimitAcrossSessions(t *testing.T) {
	t.Parallel()

	ipLimiter := NewTokenBucketRateLimiter(TokenBucketRateLimiterConfig{
		Name:              "test_ip_global",
		RequestsPerMinute: 600,
		Burst:             1,
		KeyFunc:           ClientIPRateLimitKey,
	})
	sessionLimiter := NewTokenBucketRateLimiter(TokenBucketRateLimiterConfig{
		Name:              "test_session_relaxed",
		RequestsPerMinute: 600,
		Burst:             10,
		KeyFunc:           SessionCodeRateLimitKey,
	})

	wrapped := ipLimiter.Wrap(sessionLimiter.Wrap(func(w http.ResponseWriter, _ *http.Request) {
		WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}))

	req1 := httptest.NewRequest(http.MethodPost, "/api/voice/token", nil)
	req1.RemoteAddr = "203.0.113.6:1234"
	req1 = req1.WithContext(WithSessionAuthClaims(req1.Context(), &SessionAuthClaims{SessionCode: "AP-AAAA-BBBB"}))
	rr1 := httptest.NewRecorder()
	wrapped(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("first status = %d, want 200", rr1.Code)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/api/voice/token", nil)
	req2.RemoteAddr = "203.0.113.6:1234"
	req2 = req2.WithContext(WithSessionAuthClaims(req2.Context(), &SessionAuthClaims{SessionCode: "AP-ZZZZ-YYYY"}))
	rr2 := httptest.NewRecorder()
	wrapped(rr2, req2)
	if rr2.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want 429 (global IP bucket)", rr2.Code)
	}
	if retry := rr2.Header().Get("Retry-After"); retry == "" {
		t.Fatalf("expected Retry-After header on IP limiter block")
	}
}
