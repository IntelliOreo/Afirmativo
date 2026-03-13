// SessionStore interface — defined by the consumer (this package).
// Implemented by PostgresSessionStore in postgres.go.
package session

import (
	"context"
	"time"
)

// Store defines the persistence operations for sessions and coupons.
// The implementation handles transaction management internally.
type Store interface {
	// ClaimCouponAndCreateSession atomically claims a coupon and creates a session
	// within a single database transaction. Returns the created session or an error
	// if the coupon is invalid/exhausted.
	ClaimCouponAndCreateSession(ctx context.Context, couponCode, sessionCode, pinHash string, expiresAt time.Time, interviewBudgetSeconds int) (*Session, error)

	// GetSessionByCode retrieves a session by its code.
	GetSessionByCode(ctx context.Context, sessionCode string) (*Session, error)

	// StartSession atomically transitions a session from 'created' to 'interviewing'
	// and stores preferredLanguage on first interview start.
	// Returns ErrConflict if the session is not in 'created' status.
	StartSession(ctx context.Context, sessionCode, preferredLanguage string) (*Session, error)

	// CompleteSession marks an interviewing session as completed with ended_at = now().
	CompleteSession(ctx context.Context, sessionCode string) error
}
