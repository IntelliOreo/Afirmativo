package payment

import (
	"errors"
	"time"
)

var (
	ErrInvalidStripeSignature = errors.New("invalid stripe signature")
	ErrRevealExpired          = errors.New("payment reveal expired")
	ErrRevealConsumed         = errors.New("payment reveal consumed")
	ErrReferenceMismatch      = errors.New("payment reference mismatch")
)

type Status string

const (
	StatusPending           Status = "pending"
	StatusPaidUnprovisioned Status = "paid_unprovisioned"
	StatusProvisioned       Status = "provisioned"
	StatusFailed            Status = "failed"
)

type Payment struct {
	ID                string
	CheckoutSessionID string
	SessionCode       string
	AmountCents       int
	Currency          string
	Status            Status
	RevealPIN         string
	RevealExpiresAt   *time.Time
	RevealConsumedAt  *time.Time
	FailureCode       string
	FailureDetail     string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type PaymentReference struct {
	PaymentID         string
	CheckoutSessionID string
}

type ProvisionData struct {
	SessionCode            string
	PIN                    string
	PINHash                string
	PaymentID              string
	ExpiresAt              time.Time
	RevealExpiresAt        time.Time
	InterviewBudgetSeconds int
}

type PollResult struct {
	Payment     *Payment
	SessionCode string
	PIN         string
}

type CheckoutStatus struct {
	Status      string
	SessionCode string
	PIN         string
	FailureCode string
}
