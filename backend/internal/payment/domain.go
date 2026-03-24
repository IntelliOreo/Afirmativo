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
	ErrInvalidProductType     = errors.New("invalid payment product type")
	ErrUnknownProductType     = errors.New("unknown payment product type")
)

type Status string
type ProductType string

const (
	StatusPending           Status = "pending"
	StatusPaidUnprovisioned Status = "paid_unprovisioned"
	StatusProvisioned       Status = "provisioned"
	StatusFailed            Status = "failed"

	ProductTypeDirectSession ProductType = "direct_session"
	ProductTypeCouponPack10  ProductType = "coupon_pack_10"
	couponPack10MaxUses      int         = 10
)

type Payment struct {
	ID                string
	CheckoutSessionID string
	ProductType       ProductType
	SessionCode       string
	CouponCode        string
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

type ProductConfig struct {
	AmountCents int
	Currency    string
	ProductName string
}

type FulfillmentData struct {
	ProductType            ProductType
	SessionCode            string
	PIN                    string
	PINHash                string
	PaymentID              string
	ExpiresAt              time.Time
	RevealExpiresAt        time.Time
	InterviewBudgetSeconds int
	CouponCode             string
	CouponMaxUses          int
	CouponCurrentUses      int
	CouponSource           string
}

type PollResult struct {
	Payment           *Payment
	SessionCode       string
	PIN               string
	CouponCode        string
	CouponMaxUses     int
	CouponCurrentUses int
}

type CheckoutStatus struct {
	Status            string
	ProductType       ProductType
	SessionCode       string
	PIN               string
	CouponCode        string
	CouponMaxUses     int
	CouponCurrentUses int
	FailureCode       string
}

func normalizeProductType(raw string) (ProductType, error) {
	switch ProductType(raw) {
	case "":
		return ProductTypeDirectSession, nil
	case ProductTypeDirectSession, ProductTypeCouponPack10:
		return ProductType(raw), nil
	default:
		return "", ErrInvalidProductType
	}
}
