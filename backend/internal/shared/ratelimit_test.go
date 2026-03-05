package shared

import (
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
