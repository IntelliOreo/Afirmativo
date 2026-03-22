package payment

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/afirmativo/backend/internal/shared"
	"github.com/afirmativo/backend/internal/sqlgen"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ Store = (*PostgresStore)(nil)

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

func (s *PostgresStore) CreatePendingPayment(ctx context.Context, amountCents int, currency string) (*Payment, error) {
	row, err := sqlgen.New(s.pool).CreatePendingPayment(ctx, sqlgen.CreatePendingPaymentParams{
		AmountCents: int32(amountCents),
		Currency:    currency,
	})
	if err != nil {
		return nil, fmt.Errorf("create pending payment: %w", err)
	}
	return paymentFromRow(row), nil
}

func (s *PostgresStore) AttachCheckoutSessionID(ctx context.Context, paymentID, checkoutSessionID string) (*Payment, error) {
	parsedID, err := parsePaymentID(paymentID)
	if err != nil {
		return nil, err
	}
	row, err := sqlgen.New(s.pool).AttachCheckoutSessionID(ctx, sqlgen.AttachCheckoutSessionIDParams{
		ID:                parsedID,
		CheckoutSessionID: nullableText(checkoutSessionID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		return nil, fmt.Errorf("attach checkout session id: %w", err)
	}
	return paymentFromRow(row), nil
}

func (s *PostgresStore) MarkPaymentFailed(ctx context.Context, ref PaymentReference, failureCode, failureDetail string) (*Payment, error) {
	return s.withLockedPayment(ctx, ref, func(ctx context.Context, q *sqlgen.Queries, row sqlgen.Payment) (*Payment, error) {
		updated, err := q.MarkPaymentFailedByID(ctx, sqlgen.MarkPaymentFailedByIDParams{
			ID:            row.ID,
			Column2:       checkoutSessionValue(row, ref.CheckoutSessionID),
			FailureCode:   nullableText(failureCode),
			FailureDetail: nullableText(failureDetail),
		})
		if err != nil {
			return nil, fmt.Errorf("mark payment failed: %w", err)
		}
		return paymentFromRow(updated), nil
	})
}

func (s *PostgresStore) MarkPaymentPaid(ctx context.Context, ref PaymentReference, now time.Time) (*Payment, error) {
	return s.withLockedPayment(ctx, ref, func(ctx context.Context, q *sqlgen.Queries, row sqlgen.Payment) (*Payment, error) {
		switch Status(row.Status) {
		case StatusPending:
			updated, err := q.MarkPaymentPaidUnprovisionedByID(ctx, sqlgen.MarkPaymentPaidUnprovisionedByIDParams{
				ID:        row.ID,
				Column2:   checkoutSessionValue(row, ref.CheckoutSessionID),
				UpdatedAt: pgtype.Timestamptz{Time: now, Valid: true},
			})
			if err != nil {
				return nil, fmt.Errorf("mark payment paid: %w", err)
			}
			return paymentFromRow(updated), nil
		case StatusPaidUnprovisioned, StatusProvisioned, StatusFailed:
			return paymentFromRow(row), nil
		default:
			return nil, fmt.Errorf("unsupported payment status %q", row.Status)
		}
	})
}

func (s *PostgresStore) ProvisionIfNeeded(ctx context.Context, checkoutSessionID string, now time.Time, buildProvision func() (*ProvisionData, error)) (*Payment, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	q := sqlgen.New(tx)
	row, err := q.GetPaymentByCheckoutSessionIDForUpdate(ctx, nullableText(checkoutSessionID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		return nil, fmt.Errorf("lock payment by checkout session id: %w", err)
	}

	switch Status(row.Status) {
	case StatusProvisioned, StatusPending, StatusFailed:
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit tx: %w", err)
		}
		return paymentFromRow(row), nil
	case StatusPaidUnprovisioned:
		provisionData, err := buildProvision()
		if err != nil {
			return nil, err
		}
		provisionData.PaymentID = paymentIDString(row.ID)
		if err := q.CreatePaidSession(ctx, sqlgen.CreatePaidSessionParams{
			SessionCode:            provisionData.SessionCode,
			PinHash:                provisionData.PINHash,
			PaymentID:              nullableText(provisionData.PaymentID),
			ExpiresAt:              pgtype.Timestamptz{Time: provisionData.ExpiresAt, Valid: true},
			InterviewBudgetSeconds: int32(provisionData.InterviewBudgetSeconds),
		}); err != nil {
			return nil, fmt.Errorf("create paid session: %w", err)
		}
		updated, err := q.MarkPaymentProvisioned(ctx, sqlgen.MarkPaymentProvisionedParams{
			ID:              row.ID,
			SessionCode:     nullableText(provisionData.SessionCode),
			RevealPin:       nullableText(provisionData.PIN),
			RevealExpiresAt: pgtype.Timestamptz{Time: provisionData.RevealExpiresAt, Valid: true},
			UpdatedAt:       pgtype.Timestamptz{Time: now, Valid: true},
		})
		if err != nil {
			return nil, fmt.Errorf("mark payment provisioned: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit tx: %w", err)
		}
		return paymentFromRow(updated), nil
	default:
		return nil, fmt.Errorf("unsupported payment status %q", row.Status)
	}
}

func (s *PostgresStore) ResolveCheckoutSessionForPoll(ctx context.Context, checkoutSessionID string, now time.Time, buildProvision func() (*ProvisionData, error)) (*PollResult, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	q := sqlgen.New(tx)
	row, err := q.GetPaymentByCheckoutSessionIDForUpdate(ctx, nullableText(checkoutSessionID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		return nil, fmt.Errorf("lock payment by checkout session id: %w", err)
	}

	switch Status(row.Status) {
	case StatusPending, StatusFailed:
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit tx: %w", err)
		}
		return &PollResult{Payment: paymentFromRow(row)}, nil
	case StatusPaidUnprovisioned:
		provisionData, err := buildProvision()
		if err != nil {
			return nil, err
		}
		provisionData.PaymentID = paymentIDString(row.ID)
		if err := q.CreatePaidSession(ctx, sqlgen.CreatePaidSessionParams{
			SessionCode:            provisionData.SessionCode,
			PinHash:                provisionData.PINHash,
			PaymentID:              nullableText(provisionData.PaymentID),
			ExpiresAt:              pgtype.Timestamptz{Time: provisionData.ExpiresAt, Valid: true},
			InterviewBudgetSeconds: int32(provisionData.InterviewBudgetSeconds),
		}); err != nil {
			return nil, fmt.Errorf("create paid session: %w", err)
		}
		row, err = q.MarkPaymentProvisioned(ctx, sqlgen.MarkPaymentProvisionedParams{
			ID:              row.ID,
			SessionCode:     nullableText(provisionData.SessionCode),
			RevealPin:       nullableText(provisionData.PIN),
			RevealExpiresAt: pgtype.Timestamptz{Time: provisionData.RevealExpiresAt, Valid: true},
			UpdatedAt:       pgtype.Timestamptz{Time: now, Valid: true},
		})
		if err != nil {
			return nil, fmt.Errorf("mark payment provisioned: %w", err)
		}
	case StatusProvisioned:
		// Already provisioned; fall through to reveal consumption.
	default:
		return nil, fmt.Errorf("unsupported payment status %q", row.Status)
	}

	if row.RevealConsumedAt.Valid {
		return nil, ErrRevealConsumed
	}
	if row.RevealExpiresAt.Valid && !row.RevealExpiresAt.Time.After(now) {
		return nil, ErrRevealExpired
	}
	if !row.RevealPin.Valid || !row.SessionCode.Valid {
		return nil, fmt.Errorf("payment provisioned without reveal data")
	}

	consumed, err := q.ConsumePaymentReveal(ctx, sqlgen.ConsumePaymentRevealParams{
		ID:               row.ID,
		RevealConsumedAt: pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("consume payment reveal: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}
	return &PollResult{
		Payment:     paymentFromRow(consumed),
		SessionCode: consumed.SessionCode.String,
		PIN:         consumed.RevealPin.String,
	}, nil
}

func (s *PostgresStore) MarkProvisionFailure(ctx context.Context, checkoutSessionID, failureCode, failureDetail string) (*Payment, error) {
	row, err := sqlgen.New(s.pool).MarkPaymentProvisionFailure(ctx, sqlgen.MarkPaymentProvisionFailureParams{
		CheckoutSessionID: nullableText(checkoutSessionID),
		FailureCode:       nullableText(failureCode),
		FailureDetail:     nullableText(failureDetail),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, shared.ErrNotFound
		}
		return nil, fmt.Errorf("mark provision failure: %w", err)
	}
	return paymentFromRow(row), nil
}

func (s *PostgresStore) withLockedPayment(ctx context.Context, ref PaymentReference, fn func(context.Context, *sqlgen.Queries, sqlgen.Payment) (*Payment, error)) (*Payment, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	q := sqlgen.New(tx)
	row, err := lockPayment(ctx, q, ref)
	if err != nil {
		return nil, err
	}

	if row.CheckoutSessionID.Valid && strings.TrimSpace(ref.CheckoutSessionID) != "" && row.CheckoutSessionID.String != ref.CheckoutSessionID {
		return nil, ErrReferenceMismatch
	}
	if !row.CheckoutSessionID.Valid && strings.TrimSpace(ref.CheckoutSessionID) != "" {
		row, err = q.AttachCheckoutSessionID(ctx, sqlgen.AttachCheckoutSessionIDParams{
			ID:                row.ID,
			CheckoutSessionID: nullableText(ref.CheckoutSessionID),
		})
		if err != nil {
			return nil, fmt.Errorf("attach checkout session id under lock: %w", err)
		}
	}

	result, err := fn(ctx, q, row)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}
	return result, nil
}

func lockPayment(ctx context.Context, q *sqlgen.Queries, ref PaymentReference) (sqlgen.Payment, error) {
	if strings.TrimSpace(ref.PaymentID) != "" {
		parsedID, err := parsePaymentID(ref.PaymentID)
		if err != nil {
			return sqlgen.Payment{}, err
		}
		row, err := q.GetPaymentByIDForUpdate(ctx, parsedID)
		if err == nil {
			return row, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return sqlgen.Payment{}, fmt.Errorf("lock payment by id: %w", err)
		}
		if strings.TrimSpace(ref.CheckoutSessionID) == "" {
			return sqlgen.Payment{}, shared.ErrNotFound
		}
	}

	if strings.TrimSpace(ref.CheckoutSessionID) == "" {
		return sqlgen.Payment{}, fmt.Errorf("payment lookup requires payment id or checkout session id")
	}
	row, err := q.GetPaymentByCheckoutSessionIDForUpdate(ctx, nullableText(ref.CheckoutSessionID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sqlgen.Payment{}, shared.ErrNotFound
		}
		return sqlgen.Payment{}, fmt.Errorf("lock payment by checkout session id: %w", err)
	}
	return row, nil
}

func paymentFromRow(row sqlgen.Payment) *Payment {
	payment := &Payment{
		ID:          paymentIDString(row.ID),
		AmountCents: int(row.AmountCents),
		Currency:    row.Currency,
		Status:      Status(row.Status),
		CreatedAt:   row.CreatedAt.Time,
		UpdatedAt:   row.UpdatedAt.Time,
	}
	if row.CheckoutSessionID.Valid {
		payment.CheckoutSessionID = row.CheckoutSessionID.String
	}
	if row.SessionCode.Valid {
		payment.SessionCode = row.SessionCode.String
	}
	if row.RevealPin.Valid {
		payment.RevealPIN = row.RevealPin.String
	}
	if row.RevealExpiresAt.Valid {
		t := row.RevealExpiresAt.Time
		payment.RevealExpiresAt = &t
	}
	if row.RevealConsumedAt.Valid {
		t := row.RevealConsumedAt.Time
		payment.RevealConsumedAt = &t
	}
	if row.FailureCode.Valid {
		payment.FailureCode = row.FailureCode.String
	}
	if row.FailureDetail.Valid {
		payment.FailureDetail = row.FailureDetail.String
	}
	return payment
}

func parsePaymentID(id string) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("parse payment id: %w", err)
	}
	var bytes [16]byte
	copy(bytes[:], parsed[:])
	return pgtype.UUID{Bytes: bytes, Valid: true}, nil
}

func paymentIDString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	parsed, err := uuid.FromBytes(id.Bytes[:])
	if err != nil {
		return ""
	}
	return parsed.String()
}

func nullableText(value string) pgtype.Text {
	value = strings.TrimSpace(value)
	return pgtype.Text{String: value, Valid: value != ""}
}

func checkoutSessionValue(row sqlgen.Payment, fallback string) string {
	if row.CheckoutSessionID.Valid {
		return row.CheckoutSessionID.String
	}
	return strings.TrimSpace(fallback)
}
