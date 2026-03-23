// PostgresSessionStore implements SessionStore using sqlgen + pgx.
// Maps between sqlgen-generated DB types and domain Session type.
package session

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/afirmativo/backend/internal/shared"
	"github.com/afirmativo/backend/internal/sqlgen"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore implements Store backed by PostgreSQL via sqlgen.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore creates a new PostgresStore.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

// ClaimCouponAndCreateSession runs ClaimCoupon + CreateSession in a single
// database transaction. If the coupon is invalid or exhausted, the TX is
// rolled back and ErrCouponInvalid is returned.
func (s *PostgresStore) ClaimCouponAndCreateSession(ctx context.Context, couponCode, sessionCode, pinHash string, expiresAt time.Time, interviewBudgetSeconds int) (*CouponClaimSessionResult, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback on committed tx is a no-op

	q := sqlgen.New(tx)

	// Atomically claim the coupon (UPDATE ... WHERE current_uses < max_uses).
	claimedCoupon, err := q.ClaimCoupon(ctx, couponCode)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, shared.ErrCouponInvalid
		}
		return nil, fmt.Errorf("claim coupon: %w", err)
	}

	// Create the session in the same transaction.
	row, err := q.CreateSession(ctx, sqlgen.CreateSessionParams{
		SessionCode:            sessionCode,
		PinHash:                pinHash,
		CouponCode:             pgtype.Text{String: couponCode, Valid: true},
		ExpiresAt:              pgtype.Timestamptz{Time: expiresAt, Valid: true},
		InterviewBudgetSeconds: int32(interviewBudgetSeconds),
	})
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	return &CouponClaimSessionResult{
		Session: sessionFromRow(row),
		Coupon:  couponFromRow(claimedCoupon),
	}, nil
}

// GetSessionByCode retrieves a session by its code.
func (s *PostgresStore) GetSessionByCode(ctx context.Context, sessionCode string) (*Session, error) {
	row, err := sqlgen.New(s.pool).GetSessionByCode(ctx, sessionCode)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		return nil, fmt.Errorf("get session: %w", err)
	}
	return sessionFromRow(row), nil
}

// StartSession atomically transitions a session to 'interviewing'.
func (s *PostgresStore) StartSession(ctx context.Context, sessionCode, preferredLanguage string) (*Session, error) {
	row, err := sqlgen.New(s.pool).StartSession(ctx, sqlgen.StartSessionParams{
		SessionCode:       sessionCode,
		PreferredLanguage: pgtype.Text{String: preferredLanguage, Valid: preferredLanguage != ""},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, shared.ErrConflict
		}
		return nil, fmt.Errorf("start session: %w", err)
	}
	return sessionFromRow(row), nil
}

// CompleteSession marks an interviewing session as completed.
func (s *PostgresStore) CompleteSession(ctx context.Context, sessionCode string) error {
	return sqlgen.New(s.pool).CompleteSession(ctx, sessionCode)
}

// sessionFromRow maps a sqlgen.Session to the domain Session type.
func sessionFromRow(row sqlgen.Session) *Session {
	s := &Session{
		SessionCode:            row.SessionCode,
		PinHash:                row.PinHash,
		Status:                 SessionStatus(row.Status),
		Role:                   row.Role,
		InterviewBudgetSeconds: int(row.InterviewBudgetSeconds),
		InterviewLapsedSeconds: int(row.InterviewLapsedSeconds),
		ExpiresAt:              row.ExpiresAt.Time,
		CreatedAt:              row.CreatedAt.Time,
	}
	if row.Track.Valid {
		s.Track = row.Track.String
	}
	if row.PreferredLanguage.Valid {
		s.PreferredLanguage = row.PreferredLanguage.String
	}
	if row.InterviewLapsedUpdatedAt.Valid {
		t := row.InterviewLapsedUpdatedAt.Time
		s.InterviewLapsedUpdatedAt = &t
	}
	if row.InterviewStartedAt.Valid {
		t := row.InterviewStartedAt.Time
		s.InterviewStartedAt = &t
	}
	if row.CurrentInterviewStartedAt.Valid {
		t := row.CurrentInterviewStartedAt.Time
		s.CurrentInterviewStartedAt = &t
	}
	if row.LastApiCallAt.Valid {
		t := row.LastApiCallAt.Time
		s.LastAPICallAt = &t
	}
	if row.EndedAt.Valid {
		t := row.EndedAt.Time
		s.EndedAt = &t
	}
	if len(row.ConversationHistory) > 0 {
		s.ConversationHistory = row.ConversationHistory
	}
	if row.PaymentID.Valid {
		s.PaymentID = row.PaymentID.String
	}
	if row.CouponCode.Valid {
		s.CouponCode = row.CouponCode.String
	}
	return s
}

func couponFromRow(row sqlgen.Coupon) Coupon {
	c := Coupon{
		Code:        row.Code,
		MaxUses:     int(row.MaxUses),
		CurrentUses: int(row.CurrentUses),
		DiscountPct: int(row.DiscountPct),
	}
	if row.ExpiresAt.Valid {
		t := row.ExpiresAt.Time
		c.ExpiresAt = &t
	}
	return c
}
