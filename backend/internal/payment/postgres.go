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

func (s *PostgresStore) CreatePendingPayment(ctx context.Context, amountCents int, currency string, productType ProductType) (*Payment, error) {
	row, err := sqlgen.New(s.pool).CreatePendingPayment(ctx, sqlgen.CreatePendingPaymentParams{
		AmountCents: int32(amountCents),
		Currency:    currency,
		ProductType: string(productType),
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

func (s *PostgresStore) GetPayment(ctx context.Context, ref PaymentReference) (*Payment, error) {
	return s.withLockedPayment(ctx, ref, func(_ context.Context, _ *sqlgen.Queries, row sqlgen.Payment) (*Payment, error) {
		if _, err := rowProductType(row); err != nil {
			return nil, err
		}
		return paymentFromRow(row), nil
	})
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
		if _, err := rowProductType(row); err != nil {
			return nil, err
		}

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

func (s *PostgresStore) ProvisionIfNeeded(ctx context.Context, checkoutSessionID string, now time.Time, buildFulfillment BuildFulfillmentFunc) (*Payment, error) {
	result, err := s.resolveCheckoutSession(ctx, checkoutSessionID, now, false, buildFulfillment)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("missing payment result")
	}
	return result.Payment, nil
}

func (s *PostgresStore) ResolveCheckoutSessionForPoll(ctx context.Context, checkoutSessionID string, now time.Time, buildFulfillment BuildFulfillmentFunc) (*PollResult, error) {
	return s.resolveCheckoutSession(ctx, checkoutSessionID, now, true, buildFulfillment)
}

func (s *PostgresStore) resolveCheckoutSession(ctx context.Context, checkoutSessionID string, now time.Time, forPoll bool, buildFulfillment BuildFulfillmentFunc) (*PollResult, error) {
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

	productType, err := rowProductType(row)
	if err != nil {
		return nil, err
	}

	switch Status(row.Status) {
	case StatusPending, StatusFailed:
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit tx: %w", err)
		}
		return &PollResult{Payment: paymentFromRow(row)}, nil
	case StatusPaidUnprovisioned:
		row, err = s.provisionLockedPayment(ctx, q, row, productType, now, buildFulfillment)
		if err != nil {
			return nil, err
		}
	case StatusProvisioned:
		// Already provisioned; resolve poll output below when needed.
	default:
		return nil, fmt.Errorf("unsupported payment status %q", row.Status)
	}

	if !forPoll {
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit tx: %w", err)
		}
		return &PollResult{Payment: paymentFromRow(row)}, nil
	}

	result, err := s.resolvePollResult(ctx, q, row, productType, now)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}
	return result, nil
}

func (s *PostgresStore) provisionLockedPayment(ctx context.Context, q *sqlgen.Queries, row sqlgen.Payment, productType ProductType, now time.Time, buildFulfillment BuildFulfillmentFunc) (sqlgen.Payment, error) {
	paymentID := paymentIDString(row.ID)
	fulfillment, err := buildFulfillment(productType, paymentID)
	if err != nil {
		return sqlgen.Payment{}, err
	}

	switch productType {
	case ProductTypeDirectSession:
		if err := q.CreatePaidSession(ctx, sqlgen.CreatePaidSessionParams{
			SessionCode:            fulfillment.SessionCode,
			PinHash:                fulfillment.PINHash,
			PaymentID:              nullableText(paymentID),
			ExpiresAt:              pgtype.Timestamptz{Time: fulfillment.ExpiresAt, Valid: true},
			InterviewBudgetSeconds: int32(fulfillment.InterviewBudgetSeconds),
		}); err != nil {
			return sqlgen.Payment{}, fmt.Errorf("create paid session: %w", err)
		}

		updated, err := q.MarkPaymentProvisioned(ctx, sqlgen.MarkPaymentProvisionedParams{
			ID:              row.ID,
			SessionCode:     nullableText(fulfillment.SessionCode),
			RevealPin:       nullableText(fulfillment.PIN),
			RevealExpiresAt: pgtype.Timestamptz{Time: fulfillment.RevealExpiresAt, Valid: true},
			UpdatedAt:       pgtype.Timestamptz{Time: now, Valid: true},
		})
		if err != nil {
			return sqlgen.Payment{}, fmt.Errorf("mark payment provisioned: %w", err)
		}
		return updated, nil
	case ProductTypeCouponPack10:
		createdCoupon, err := q.CreateCoupon(ctx, sqlgen.CreateCouponParams{
			Code:    fulfillment.CouponCode,
			MaxUses: int32(fulfillment.CouponMaxUses),
			Source:  nullableText(fulfillment.CouponSource),
		})
		if err != nil {
			return sqlgen.Payment{}, fmt.Errorf("create coupon: %w", err)
		}

		updated, err := q.MarkPaymentProvisionedCouponPack(ctx, sqlgen.MarkPaymentProvisionedCouponPackParams{
			ID:         row.ID,
			CouponCode: nullableText(createdCoupon.Code),
			UpdatedAt:  pgtype.Timestamptz{Time: now, Valid: true},
		})
		if err != nil {
			return sqlgen.Payment{}, fmt.Errorf("mark payment provisioned coupon pack: %w", err)
		}
		return updated, nil
	default:
		return sqlgen.Payment{}, fmt.Errorf("%w: %q", ErrUnknownProductType, productType)
	}
}

func (s *PostgresStore) resolvePollResult(ctx context.Context, q *sqlgen.Queries, row sqlgen.Payment, productType ProductType, now time.Time) (*PollResult, error) {
	switch productType {
	case ProductTypeDirectSession:
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
		return &PollResult{
			Payment:     paymentFromRow(consumed),
			SessionCode: consumed.SessionCode.String,
			PIN:         consumed.RevealPin.String,
		}, nil
	case ProductTypeCouponPack10:
		if !row.CouponCode.Valid {
			return nil, fmt.Errorf("coupon-pack payment provisioned without coupon code")
		}
		return &PollResult{
			Payment:           paymentFromRow(row),
			CouponCode:        row.CouponCode.String,
			CouponMaxUses:     couponPack10MaxUses,
			CouponCurrentUses: 0,
		}, nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownProductType, productType)
	}
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
		ProductType: ProductType(row.ProductType),
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
	if row.CouponCode.Valid {
		payment.CouponCode = row.CouponCode.String
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

func rowProductType(row sqlgen.Payment) (ProductType, error) {
	switch ProductType(row.ProductType) {
	case ProductTypeDirectSession, ProductTypeCouponPack10:
		return ProductType(row.ProductType), nil
	default:
		return "", fmt.Errorf("%w: %q", ErrUnknownProductType, row.ProductType)
	}
}
