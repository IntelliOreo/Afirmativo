package payment

import (
	"context"
	"time"
)

type Store interface {
	CreatePendingPayment(ctx context.Context, amountCents int, currency string) (*Payment, error)
	AttachCheckoutSessionID(ctx context.Context, paymentID, checkoutSessionID string) (*Payment, error)
	MarkPaymentFailed(ctx context.Context, ref PaymentReference, failureCode, failureDetail string) (*Payment, error)
	MarkPaymentPaid(ctx context.Context, ref PaymentReference, now time.Time) (*Payment, error)
	ProvisionIfNeeded(ctx context.Context, checkoutSessionID string, now time.Time, buildProvision func() (*ProvisionData, error)) (*Payment, error)
	ResolveCheckoutSessionForPoll(ctx context.Context, checkoutSessionID string, now time.Time, buildProvision func() (*ProvisionData, error)) (*PollResult, error)
	MarkProvisionFailure(ctx context.Context, checkoutSessionID, failureCode, failureDetail string) (*Payment, error)
}
