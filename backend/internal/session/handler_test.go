package session

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/afirmativo/backend/internal/shared"
	"golang.org/x/crypto/bcrypt"
)

type fakeStore struct {
	claimCouponAndCreateSessionFn func(ctx context.Context, couponCode, sessionCode, pinHash string, expiresAt time.Time) (*Session, error)
	getSessionByCodeFn            func(ctx context.Context, sessionCode string) (*Session, error)
}

func (f *fakeStore) ClaimCouponAndCreateSession(ctx context.Context, couponCode, sessionCode, pinHash string, expiresAt time.Time) (*Session, error) {
	if f.claimCouponAndCreateSessionFn != nil {
		return f.claimCouponAndCreateSessionFn(ctx, couponCode, sessionCode, pinHash, expiresAt)
	}
	return nil, nil
}

func (f *fakeStore) GetSessionByCode(ctx context.Context, sessionCode string) (*Session, error) {
	if f.getSessionByCodeFn != nil {
		return f.getSessionByCodeFn(ctx, sessionCode)
	}
	return nil, nil
}

func (f *fakeStore) StartSession(context.Context, string, string) (*Session, error) { return nil, nil }
func (f *fakeStore) CompleteSession(context.Context, string) error                  { return nil }

func hashPIN(t *testing.T, pin string) string {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(pin), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("bcrypt.GenerateFromPassword() error = %v", err)
	}
	return string(h)
}

func newHandlerForTest(t *testing.T, store Store) *Handler {
	t.Helper()
	auth, err := shared.NewSessionAuthManager(shared.SessionAuthConfig{
		Secret:       "test-session-auth-secret-32-bytes-minimum",
		CookieName:   "test_session_auth",
		Issuer:       "afirmativo-test",
		Audience:     "afirmativo-test-ui",
		CookieSecure: false,
	})
	if err != nil {
		t.Fatalf("NewSessionAuthManager() error = %v", err)
	}
	return NewHandler(NewService(store, 24), auth, time.Hour)
}

func decodeJSONBody(t *testing.T, rr *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.Unmarshal(rr.Body.Bytes(), dst); err != nil {
		t.Fatalf("json.Unmarshal() error = %v; body=%s", err, rr.Body.String())
	}
}

func TestHandleValidateCoupon_MapsCouponInvalid(t *testing.T) {
	t.Parallel()

	h := newHandlerForTest(t, &fakeStore{
		claimCouponAndCreateSessionFn: func(context.Context, string, string, string, time.Time) (*Session, error) {
			return nil, shared.ErrCouponInvalid
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/coupon/validate", strings.NewReader(`{"code":"BETA-0001"}`))
	rr := httptest.NewRecorder()

	h.HandleValidateCoupon(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}

	var got struct {
		Valid bool   `json:"valid"`
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	decodeJSONBody(t, rr, &got)

	if got.Valid {
		t.Fatalf("valid = %v, want false", got.Valid)
	}
	if got.Code != "COUPON_INVALID" {
		t.Fatalf("code = %q, want COUPON_INVALID", got.Code)
	}
}

func TestHandleValidateCoupon_SuccessContract(t *testing.T) {
	t.Parallel()

	var capturedExpiry time.Time
	var capturedSessionCode string
	var capturedPinHash string
	start := time.Now()

	h := newHandlerForTest(t, &fakeStore{
		claimCouponAndCreateSessionFn: func(_ context.Context, _ string, sessionCode, pinHash string, expiresAt time.Time) (*Session, error) {
			capturedSessionCode = sessionCode
			capturedPinHash = pinHash
			capturedExpiry = expiresAt
			return &Session{SessionCode: sessionCode}, nil
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/coupon/validate", strings.NewReader(`{"code":"BETA-0001"}`))
	rr := httptest.NewRecorder()
	h.HandleValidateCoupon(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	var got struct {
		Valid       bool   `json:"valid"`
		SessionCode string `json:"session_code"`
		PIN         string `json:"pin"`
	}
	decodeJSONBody(t, rr, &got)

	if !got.Valid {
		t.Fatalf("valid = %v, want true", got.Valid)
	}
	codeRe := regexp.MustCompile(`^AP-[A-HJ-NP-Z2-9]{4}-[A-HJ-NP-Z2-9]{4}$`)
	if !codeRe.MatchString(got.SessionCode) {
		t.Fatalf("session_code = %q, want AP-XXXX-XXXX", got.SessionCode)
	}
	pinRe := regexp.MustCompile(`^[0-9]{6}$`)
	if !pinRe.MatchString(got.PIN) {
		t.Fatalf("pin = %q, want 6 digits", got.PIN)
	}
	if capturedSessionCode != got.SessionCode {
		t.Fatalf("store session_code = %q, response session_code = %q", capturedSessionCode, got.SessionCode)
	}
	if capturedPinHash == got.PIN {
		t.Fatalf("store pin hash should not equal plaintext pin")
	}

	after := time.Now()
	min := start.Add(23*time.Hour + 59*time.Minute)
	max := after.Add(24*time.Hour + time.Minute)
	if capturedExpiry.Before(min) || capturedExpiry.After(max) {
		t.Fatalf("expiresAt = %s, want close to now+24h (between %s and %s)", capturedExpiry, min, max)
	}
}

func TestHandleVerifySession_ErrorMappings(t *testing.T) {
	t.Parallel()

	now := time.Now()
	validPINHash := hashPIN(t, "482917")

	tests := []struct {
		name       string
		store      *fakeStore
		body       string
		wantStatus int
		wantCode   string
	}{
		{
			name: "session_not_found",
			store: &fakeStore{
				getSessionByCodeFn: func(context.Context, string) (*Session, error) {
					return nil, shared.ErrNotFound
				},
			},
			body:       `{"sessionCode":"AP-AAAA-BBBB","pin":"482917"}`,
			wantStatus: http.StatusNotFound,
			wantCode:   "SESSION_NOT_FOUND",
		},
		{
			name: "incorrect_pin",
			store: &fakeStore{
				getSessionByCodeFn: func(context.Context, string) (*Session, error) {
					return &Session{SessionCode: "AP-AAAA-BBBB", PinHash: validPINHash, ExpiresAt: now.Add(2 * time.Hour)}, nil
				},
			},
			body:       `{"sessionCode":"AP-AAAA-BBBB","pin":"000000"}`,
			wantStatus: http.StatusUnauthorized,
			wantCode:   "PIN_INCORRECT",
		},
		{
			name: "expired",
			store: &fakeStore{
				getSessionByCodeFn: func(context.Context, string) (*Session, error) {
					return &Session{SessionCode: "AP-AAAA-BBBB", PinHash: validPINHash, ExpiresAt: now.Add(-time.Minute)}, nil
				},
			},
			body:       `{"sessionCode":"AP-AAAA-BBBB","pin":"482917"}`,
			wantStatus: http.StatusGone,
			wantCode:   "SESSION_EXPIRED",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := newHandlerForTest(t, tc.store)
			req := httptest.NewRequest(http.MethodPost, "/api/session/verify", strings.NewReader(tc.body))
			rr := httptest.NewRecorder()
			h.HandleVerifySession(rr, req)

			if rr.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rr.Code, tc.wantStatus)
			}
			var got shared.ErrorResponse
			decodeJSONBody(t, rr, &got)
			if got.Code != tc.wantCode {
				t.Fatalf("code = %q, want %q", got.Code, tc.wantCode)
			}
		})
	}
}

func TestHandleVerifySession_SuccessContract(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	startedAt := now.Add(-5 * time.Minute).Truncate(time.Second)
	createdAt := now.Add(-10 * time.Minute).Truncate(time.Second)
	expiresAt := now.Add(24 * time.Hour).Truncate(time.Second)

	h := newHandlerForTest(t, &fakeStore{
		getSessionByCodeFn: func(context.Context, string) (*Session, error) {
			return &Session{
				SessionCode:            "AP-7K9X-M2NF",
				PinHash:                hashPIN(t, "482917"),
				Status:                 "interviewing",
				Track:                  "A",
				InterviewBudgetSeconds: 3600,
				InterviewLapsedSeconds: 0,
				InterviewStartedAt:     &startedAt,
				CreatedAt:              createdAt,
				ExpiresAt:              expiresAt,
			}, nil
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/session/verify", strings.NewReader(`{"sessionCode":"AP-7K9X-M2NF","pin":"482917"}`))
	rr := httptest.NewRecorder()
	h.HandleVerifySession(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	foundAuthCookie := false
	for _, cookie := range rr.Result().Cookies() {
		if cookie.Name == "test_session_auth" && cookie.Value != "" {
			foundAuthCookie = true
			break
		}
	}
	if !foundAuthCookie {
		t.Fatalf("expected test_session_auth cookie to be set")
	}

	var got struct {
		Session struct {
			SessionCode            string     `json:"session_code"`
			Status                 string     `json:"status"`
			Track                  string     `json:"track"`
			InterviewBudgetSeconds int        `json:"interview_budget_seconds"`
			InterviewLapsedSeconds int        `json:"interview_lapsed_seconds"`
			InterviewStartedAt     *time.Time `json:"interview_started_at"`
		} `json:"session"`
	}
	decodeJSONBody(t, rr, &got)

	if got.Session.SessionCode != "AP-7K9X-M2NF" {
		t.Fatalf("session_code = %q, want AP-7K9X-M2NF", got.Session.SessionCode)
	}
	if got.Session.Status != "interviewing" {
		t.Fatalf("status = %q, want interviewing", got.Session.Status)
	}
	if got.Session.Track != "A" {
		t.Fatalf("track = %q, want A", got.Session.Track)
	}
	if got.Session.InterviewBudgetSeconds != 3600 {
		t.Fatalf("interview_budget_seconds = %d, want 3600", got.Session.InterviewBudgetSeconds)
	}
	if got.Session.InterviewStartedAt == nil || !got.Session.InterviewStartedAt.Equal(startedAt) {
		t.Fatalf("interview_started_at = %v, want %v", got.Session.InterviewStartedAt, startedAt)
	}
}

func TestHandleVerifySession_LocksOutAfterTooManyFailures(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	h := newHandlerForTest(t, &fakeStore{
		getSessionByCodeFn: func(context.Context, string) (*Session, error) {
			return &Session{
				SessionCode: "AP-AAAA-BBBB",
				PinHash:     hashPIN(t, "482917"),
				ExpiresAt:   now.Add(2 * time.Hour),
			}, nil
		},
	})
	h.SetVerifyAttemptLimiter(shared.NewFailedAttemptLockoutLimiter(shared.FailedAttemptLockoutConfig{
		Name:        "test_verify_lockout",
		MaxFailures: 2,
		Window:      10 * time.Minute,
		Lockout:     15 * time.Minute,
	}))

	firstReq := httptest.NewRequest(http.MethodPost, "/api/session/verify", strings.NewReader(`{"sessionCode":"AP-AAAA-BBBB","pin":"000000"}`))
	firstReq.RemoteAddr = "203.0.113.10:1234"
	firstRR := httptest.NewRecorder()
	h.HandleVerifySession(firstRR, firstReq)
	if firstRR.Code != http.StatusUnauthorized {
		t.Fatalf("first status = %d, want %d", firstRR.Code, http.StatusUnauthorized)
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/api/session/verify", strings.NewReader(`{"sessionCode":"AP-AAAA-BBBB","pin":"000000"}`))
	secondReq.RemoteAddr = "203.0.113.10:1234"
	secondRR := httptest.NewRecorder()
	h.HandleVerifySession(secondRR, secondReq)
	if secondRR.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want %d", secondRR.Code, http.StatusTooManyRequests)
	}
	if retry := secondRR.Header().Get("Retry-After"); retry == "" {
		t.Fatalf("expected Retry-After header on lockout response")
	}
	var secondErr shared.ErrorResponse
	decodeJSONBody(t, secondRR, &secondErr)
	if secondErr.Code != "VERIFY_RATE_LIMITED" {
		t.Fatalf("second code = %q, want VERIFY_RATE_LIMITED", secondErr.Code)
	}

	thirdReq := httptest.NewRequest(http.MethodPost, "/api/session/verify", strings.NewReader(`{"sessionCode":"AP-AAAA-BBBB","pin":"482917"}`))
	thirdReq.RemoteAddr = "203.0.113.10:1234"
	thirdRR := httptest.NewRecorder()
	h.HandleVerifySession(thirdRR, thirdReq)
	if thirdRR.Code != http.StatusTooManyRequests {
		t.Fatalf("third status = %d, want %d", thirdRR.Code, http.StatusTooManyRequests)
	}
}

func TestHandleVerifySession_SuccessResetsFailureCount(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	h := newHandlerForTest(t, &fakeStore{
		getSessionByCodeFn: func(context.Context, string) (*Session, error) {
			return &Session{
				SessionCode: "AP-AAAA-BBBB",
				PinHash:     hashPIN(t, "482917"),
				ExpiresAt:   now.Add(2 * time.Hour),
			}, nil
		},
	})
	h.SetVerifyAttemptLimiter(shared.NewFailedAttemptLockoutLimiter(shared.FailedAttemptLockoutConfig{
		Name:        "test_verify_reset",
		MaxFailures: 2,
		Window:      10 * time.Minute,
		Lockout:     15 * time.Minute,
	}))

	badReq := httptest.NewRequest(http.MethodPost, "/api/session/verify", strings.NewReader(`{"sessionCode":"AP-AAAA-BBBB","pin":"000000"}`))
	badReq.RemoteAddr = "203.0.113.11:1234"
	badRR := httptest.NewRecorder()
	h.HandleVerifySession(badRR, badReq)
	if badRR.Code != http.StatusUnauthorized {
		t.Fatalf("bad status = %d, want %d", badRR.Code, http.StatusUnauthorized)
	}

	goodReq := httptest.NewRequest(http.MethodPost, "/api/session/verify", strings.NewReader(`{"sessionCode":"AP-AAAA-BBBB","pin":"482917"}`))
	goodReq.RemoteAddr = "203.0.113.11:1234"
	goodRR := httptest.NewRecorder()
	h.HandleVerifySession(goodRR, goodReq)
	if goodRR.Code != http.StatusOK {
		t.Fatalf("good status = %d, want %d", goodRR.Code, http.StatusOK)
	}

	badAgainReq := httptest.NewRequest(http.MethodPost, "/api/session/verify", strings.NewReader(`{"sessionCode":"AP-AAAA-BBBB","pin":"000000"}`))
	badAgainReq.RemoteAddr = "203.0.113.11:1234"
	badAgainRR := httptest.NewRecorder()
	h.HandleVerifySession(badAgainRR, badAgainReq)
	if badAgainRR.Code != http.StatusUnauthorized {
		t.Fatalf("bad-again status = %d, want %d", badAgainRR.Code, http.StatusUnauthorized)
	}
}

func TestHandleVerifySession_FailedAttemptLimiterIsScopedBySessionAndIP(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	h := newHandlerForTest(t, &fakeStore{
		getSessionByCodeFn: func(context.Context, string) (*Session, error) {
			return &Session{
				SessionCode: "AP-AAAA-BBBB",
				PinHash:     hashPIN(t, "482917"),
				ExpiresAt:   now.Add(2 * time.Hour),
			}, nil
		},
	})
	h.SetVerifyAttemptLimiter(shared.NewFailedAttemptLockoutLimiter(shared.FailedAttemptLockoutConfig{
		Name:        "test_verify_key_scope",
		MaxFailures: 2,
		Window:      10 * time.Minute,
		Lockout:     15 * time.Minute,
	}))

	// Lock out one specific (sessionCode + IP) key.
	first := httptest.NewRequest(http.MethodPost, "/api/session/verify", strings.NewReader(`{"sessionCode":"AP-AAAA-BBBB","pin":"000000"}`))
	first.RemoteAddr = "203.0.113.50:1234"
	firstRR := httptest.NewRecorder()
	h.HandleVerifySession(firstRR, first)
	if firstRR.Code != http.StatusUnauthorized {
		t.Fatalf("first status = %d, want %d", firstRR.Code, http.StatusUnauthorized)
	}

	second := httptest.NewRequest(http.MethodPost, "/api/session/verify", strings.NewReader(`{"sessionCode":"AP-AAAA-BBBB","pin":"000000"}`))
	second.RemoteAddr = "203.0.113.50:1234"
	secondRR := httptest.NewRecorder()
	h.HandleVerifySession(secondRR, second)
	if secondRR.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want %d", secondRR.Code, http.StatusTooManyRequests)
	}

	// Same session, different IP should not inherit lockout.
	otherIP := httptest.NewRequest(http.MethodPost, "/api/session/verify", strings.NewReader(`{"sessionCode":"AP-AAAA-BBBB","pin":"000000"}`))
	otherIP.RemoteAddr = "203.0.113.51:1234"
	otherIPRR := httptest.NewRecorder()
	h.HandleVerifySession(otherIPRR, otherIP)
	if otherIPRR.Code != http.StatusUnauthorized {
		t.Fatalf("other-ip status = %d, want %d", otherIPRR.Code, http.StatusUnauthorized)
	}

	// Same IP, different session should not inherit lockout.
	otherSession := httptest.NewRequest(http.MethodPost, "/api/session/verify", strings.NewReader(`{"sessionCode":"AP-CCCC-DDDD","pin":"000000"}`))
	otherSession.RemoteAddr = "203.0.113.50:1234"
	otherSessionRR := httptest.NewRecorder()
	h.HandleVerifySession(otherSessionRR, otherSession)
	if otherSessionRR.Code != http.StatusUnauthorized {
		t.Fatalf("other-session status = %d, want %d", otherSessionRR.Code, http.StatusUnauthorized)
	}
}

func TestHandleVerifySession_PerIPTokenBucketLimiter(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	h := newHandlerForTest(t, &fakeStore{
		getSessionByCodeFn: func(_ context.Context, sessionCode string) (*Session, error) {
			return &Session{
				SessionCode: sessionCode,
				PinHash:     hashPIN(t, "482917"),
				ExpiresAt:   now.Add(2 * time.Hour),
			}, nil
		},
	})

	ipLimiter := shared.NewTokenBucketRateLimiter(shared.TokenBucketRateLimiterConfig{
		Name:              "test_verify_ip_limiter",
		RequestsPerMinute: 1,
		Burst:             1,
		KeyFunc:           shared.ClientIPRateLimitKey,
	})
	wrapped := ipLimiter.Wrap(h.HandleVerifySession)

	first := httptest.NewRequest(http.MethodPost, "/api/session/verify", strings.NewReader(`{"sessionCode":"AP-AAAA-BBBB","pin":"482917"}`))
	first.RemoteAddr = "203.0.113.60:1234"
	firstRR := httptest.NewRecorder()
	wrapped(firstRR, first)
	if firstRR.Code != http.StatusOK {
		t.Fatalf("first status = %d, want %d", firstRR.Code, http.StatusOK)
	}

	second := httptest.NewRequest(http.MethodPost, "/api/session/verify", strings.NewReader(`{"sessionCode":"AP-AAAA-BBBB","pin":"482917"}`))
	second.RemoteAddr = "203.0.113.60:1234"
	secondRR := httptest.NewRecorder()
	wrapped(secondRR, second)
	if secondRR.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want %d", secondRR.Code, http.StatusTooManyRequests)
	}
	if retry := secondRR.Header().Get("Retry-After"); retry == "" {
		t.Fatalf("expected Retry-After header on IP limiter response")
	}
	var secondErr shared.ErrorResponse
	decodeJSONBody(t, secondRR, &secondErr)
	if secondErr.Code != "RATE_LIMITED" {
		t.Fatalf("second code = %q, want RATE_LIMITED", secondErr.Code)
	}
}
