// Service layer for session operations.
// ValidateCoupon: atomic TX — ClaimCoupon + CreateSession in one transaction.
// VerifySession: verify session code + PIN (bcrypt compare).
package session

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Service contains session business logic.
type Service struct {
	store     Store
	settings  Settings
	nowFn     func() time.Time
	dbTimeout time.Duration
}

type Deps struct {
	Store Store
}

type Settings struct {
	ExpiryHours            int
	InterviewBudgetSeconds int
	DBTimeout              time.Duration
}

// NewService creates a Service with the given dependencies and runtime settings.
func NewService(deps Deps, settings Settings) *Service {
	dbTimeout := settings.DBTimeout
	if dbTimeout <= 0 {
		dbTimeout = 5 * time.Second
	}
	return &Service{
		store:     deps.Store,
		settings:  settings,
		nowFn:     time.Now,
		dbTimeout: dbTimeout,
	}
}

// ValidateCouponResult holds the output of a successful coupon validation.
type ValidateCouponResult struct {
	SessionCode string
	PIN         string // plaintext — returned once, never stored
	Coupon      Coupon
}

// ValidateCoupon claims a coupon and creates a session atomically.
// Returns the session code and plaintext PIN (returned to the caller once).
func (s *Service) ValidateCoupon(ctx context.Context, couponCode string) (*ValidateCouponResult, error) {
	sessionCode, err := GenerateSessionCode()
	if err != nil {
		return nil, fmt.Errorf("generate session code: %w", err)
	}

	pin, err := GeneratePIN()
	if err != nil {
		return nil, fmt.Errorf("generate PIN: %w", err)
	}

	pinHash, err := bcrypt.GenerateFromPassword([]byte(pin), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash PIN: %w", err)
	}

	expiresAt := s.nowFn().Add(time.Duration(s.settings.ExpiryHours) * time.Hour)

	dbCtx, cancel := context.WithTimeout(ctx, s.dbTimeout)
	claimResult, err := s.store.ClaimCouponAndCreateSession(
		dbCtx,
		couponCode,
		sessionCode,
		string(pinHash),
		expiresAt,
		s.settings.InterviewBudgetSeconds,
	)
	cancel()
	if err != nil {
		return nil, err // ErrCouponInvalid or internal error — caller maps to HTTP status
	}

	slog.Info("session created via coupon",
		"session_code", sessionCode,
		"coupon_code", couponCode,
	)

	return &ValidateCouponResult{
		SessionCode: claimResult.Session.SessionCode,
		PIN:         pin,
		Coupon:      claimResult.Coupon,
	}, nil
}

// VerifySession verifies a session code and PIN, returning the session if valid.
func (s *Service) VerifySession(ctx context.Context, sessionCode, pin string) (*Session, error) {
	dbCtx, cancel := context.WithTimeout(ctx, s.dbTimeout)
	sess, err := s.store.GetSessionByCode(dbCtx, sessionCode)
	cancel()
	if err != nil {
		return nil, err // ErrNotFound or internal error
	}

	if s.nowFn().After(sess.ExpiresAt) {
		return nil, fmt.Errorf("%w: session expired", ErrSessionExpired)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(sess.PinHash), []byte(pin)); err != nil {
		return nil, ErrPINIncorrect
	}

	return sess, nil
}
