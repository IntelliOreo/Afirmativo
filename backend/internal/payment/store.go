package payment

import (
	"context"
	"time"
)

type BuildFulfillmentFunc func(productType ProductType, paymentID string) (*FulfillmentData, error)

type Store interface {
	CreatePendingPayment(ctx context.Context, amountCents int, currency string, productType ProductType) (*Payment, error)
	AttachCheckoutSessionID(ctx context.Context, paymentID, checkoutSessionID string) (*Payment, error)
	GetPayment(ctx context.Context, ref PaymentReference) (*Payment, error)
	MarkPaymentFailed(ctx context.Context, ref PaymentReference, failureCode, failureDetail string) (*Payment, error)
	MarkPaymentPaid(ctx context.Context, ref PaymentReference, now time.Time) (*Payment, error)
	ProvisionIfNeeded(ctx context.Context, checkoutSessionID string, now time.Time, buildFulfillment BuildFulfillmentFunc) (*Payment, error)
	ResolveCheckoutSessionForPoll(ctx context.Context, checkoutSessionID string, now time.Time, buildFulfillment BuildFulfillmentFunc) (*PollResult, error)
	MarkProvisionFailure(ctx context.Context, checkoutSessionID, failureCode, failureDetail string) (*Payment, error)
}
