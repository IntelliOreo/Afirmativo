package payment

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/afirmativo/backend/internal/session"
	"github.com/afirmativo/backend/internal/shared"
	"golang.org/x/crypto/bcrypt"
)

const (
	defaultRevealPINTTL              = 10 * time.Minute
	stripeCheckoutSessionPlaceholder = "{CHECKOUT_SESSION_ID}"
)

type Deps struct {
	Store  Store
	Stripe *StripeClient
}

type Settings struct {
	FrontendURL            string
	SessionExpiryHours     int
	InterviewBudgetSeconds int
	AmountCents            int
	Currency               string
	RevealPINTTL           time.Duration
}

type Service struct {
	store    Store
	stripe   *StripeClient
	settings Settings
	nowFn    func() time.Time
}

func NewService(deps Deps, settings Settings) *Service {
	if settings.RevealPINTTL <= 0 {
		settings.RevealPINTTL = defaultRevealPINTTL
	}
	return &Service{
		store:    deps.Store,
		stripe:   deps.Stripe,
		settings: settings,
		nowFn:    time.Now,
	}
}

func (s *Service) CreateCheckout(ctx context.Context, lang string) (*CreatedCheckoutSession, error) {
	normalizedLang, err := normalizeLang(lang)
	if err != nil {
		return nil, err
	}

	paymentRow, err := s.store.CreatePendingPayment(ctx, s.settings.AmountCents, s.settings.Currency)
	if err != nil {
		return nil, fmt.Errorf("create pending payment: %w", err)
	}

	checkout, err := s.stripe.CreateCheckoutSession(ctx, CreateCheckoutSessionParams{
		AmountCents:       s.settings.AmountCents,
		Currency:          s.settings.Currency,
		SuccessURL:        buildFrontendURL(s.settings.FrontendURL, "/pay/success", map[string]string{"session_id": stripeCheckoutSessionPlaceholder, "lang": normalizedLang}),
		CancelURL:         buildFrontendURL(s.settings.FrontendURL, "/pay", map[string]string{"lang": normalizedLang}),
		ClientReferenceID: paymentRow.ID,
	})
	if err != nil {
		_, _ = s.store.MarkPaymentFailed(ctx, PaymentReference{PaymentID: paymentRow.ID}, "STRIPE_CHECKOUT_CREATE_FAILED", truncateFailureDetail(err))
		return nil, err
	}

	if _, err := s.store.AttachCheckoutSessionID(ctx, paymentRow.ID, checkout.ID); err != nil {
		_, _ = s.store.MarkPaymentFailed(ctx, PaymentReference{PaymentID: paymentRow.ID, CheckoutSessionID: checkout.ID}, "PAYMENT_LINK_FAILED", truncateFailureDetail(err))
		return nil, fmt.Errorf("attach checkout session id: %w", err)
	}

	return checkout, nil
}

func (s *Service) HandleWebhook(ctx context.Context, payload []byte, signatureHeader string) error {
	event, err := s.stripe.ParseWebhookEvent(payload, signatureHeader)
	if err != nil {
		return err
	}
	if event == nil || event.Type != "checkout.session.completed" || event.CheckoutSession == nil {
		return nil
	}

	checkout := event.CheckoutSession
	ref := PaymentReference{
		PaymentID:         strings.TrimSpace(checkout.ClientReferenceID),
		CheckoutSessionID: strings.TrimSpace(checkout.ID),
	}

	if !amountCurrencyMatch(checkout, s.settings.AmountCents, s.settings.Currency) {
		if _, markErr := s.store.MarkPaymentFailed(ctx, ref, "PAYMENT_AMOUNT_MISMATCH", "Stripe checkout amount or currency did not match configured values"); markErr != nil {
			return fmt.Errorf("mark payment failed after amount mismatch: %w", markErr)
		}
		return nil
	}

	paymentRow, err := s.store.MarkPaymentPaid(ctx, ref, s.nowFn().UTC())
	if err != nil {
		return fmt.Errorf("mark payment paid: %w", err)
	}
	if paymentRow.Status != StatusPaidUnprovisioned {
		return nil
	}

	if _, err := s.store.ProvisionIfNeeded(ctx, checkout.ID, s.nowFn().UTC(), s.buildProvisionData); err != nil {
		_, _ = s.store.MarkProvisionFailure(ctx, checkout.ID, "SESSION_PROVISION_FAILED", truncateFailureDetail(err))
		return fmt.Errorf("provision checkout session: %w", err)
	}
	return nil
}

func (s *Service) GetCheckoutStatus(ctx context.Context, checkoutSessionID string) (*CheckoutStatus, error) {
	result, err := s.store.ResolveCheckoutSessionForPoll(ctx, strings.TrimSpace(checkoutSessionID), s.nowFn().UTC(), s.buildProvisionData)
	if err != nil {
		switch {
		case errors.Is(err, shared.ErrNotFound), errors.Is(err, ErrRevealExpired), errors.Is(err, ErrRevealConsumed):
			return nil, err
		default:
			_, _ = s.store.MarkProvisionFailure(ctx, checkoutSessionID, "SESSION_PROVISION_FAILED", truncateFailureDetail(err))
			return &CheckoutStatus{Status: "pending"}, nil
		}
	}
	if result == nil || result.Payment == nil {
		return nil, fmt.Errorf("missing checkout status result")
	}

	switch result.Payment.Status {
	case StatusPending, StatusPaidUnprovisioned:
		return &CheckoutStatus{Status: "pending"}, nil
	case StatusFailed:
		return &CheckoutStatus{Status: "failed", FailureCode: result.Payment.FailureCode}, nil
	case StatusProvisioned:
		return &CheckoutStatus{
			Status:      "ready",
			SessionCode: result.SessionCode,
			PIN:         result.PIN,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported payment status %q", result.Payment.Status)
	}
}

func (s *Service) buildProvisionData() (*ProvisionData, error) {
	sessionCode, err := session.GenerateSessionCode()
	if err != nil {
		return nil, fmt.Errorf("generate session code: %w", err)
	}
	pin, err := session.GeneratePIN()
	if err != nil {
		return nil, fmt.Errorf("generate pin: %w", err)
	}
	pinHash, err := bcrypt.GenerateFromPassword([]byte(pin), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash pin: %w", err)
	}

	now := s.nowFn().UTC()
	return &ProvisionData{
		SessionCode:            sessionCode,
		PIN:                    pin,
		PINHash:                string(pinHash),
		ExpiresAt:              now.Add(time.Duration(s.settings.SessionExpiryHours) * time.Hour),
		RevealExpiresAt:        now.Add(s.settings.RevealPINTTL),
		InterviewBudgetSeconds: s.settings.InterviewBudgetSeconds,
	}, nil
}

func normalizeLang(lang string) (string, error) {
	normalized := strings.TrimSpace(strings.ToLower(lang))
	switch normalized {
	case "en", "es":
		return normalized, nil
	default:
		return "", fmt.Errorf("%w: lang must be en or es", shared.ErrBadRequest)
	}
}

func buildFrontendURL(base, path string, query map[string]string) string {
	parsed, err := url.Parse(strings.TrimRight(base, "/"))
	if err != nil {
		return strings.TrimRight(base, "/") + path
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + path
	values := parsed.Query()
	for key, value := range query {
		values.Set(key, value)
	}
	parsed.RawQuery = strings.ReplaceAll(values.Encode(), url.QueryEscape(stripeCheckoutSessionPlaceholder), stripeCheckoutSessionPlaceholder)
	return parsed.String()
}

func amountCurrencyMatch(checkout *StripeCheckoutSession, amountCents int, currency string) bool {
	if checkout == nil {
		return false
	}
	if checkout.AmountTotal != amountCents {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(checkout.Currency), strings.TrimSpace(currency))
}

func truncateFailureDetail(err error) string {
	const maxFailureDetailLen = 500
	if err == nil {
		return ""
	}
	detail := strings.TrimSpace(err.Error())
	if len(detail) <= maxFailureDetailLen {
		return detail
	}
	return detail[:maxFailureDetailLen]
}
