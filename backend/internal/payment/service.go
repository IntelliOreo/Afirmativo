package payment

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
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
	couponAlphabet                   = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	couponPack10Prefix               = "PACK10"
	couponTokenLength                = 8
)

type Deps struct {
	Store  Store
	Stripe *StripeClient
}

type Settings struct {
	FrontendURL            string
	SessionExpiryHours     int
	InterviewBudgetSeconds int
	DirectSession          ProductConfig
	CouponPack10           ProductConfig
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

func (s *Service) CreateCheckout(ctx context.Context, lang string, rawProduct string) (*CreatedCheckoutSession, error) {
	normalizedLang, err := normalizeLang(lang)
	if err != nil {
		return nil, err
	}

	productType, err := normalizeProductType(strings.TrimSpace(strings.ToLower(rawProduct)))
	if err != nil {
		return nil, fmt.Errorf("%w: unsupported product", shared.ErrBadRequest)
	}
	productCfg, err := s.productConfig(productType)
	if err != nil {
		return nil, fmt.Errorf("%w: unsupported product", shared.ErrBadRequest)
	}

	paymentRow, err := s.store.CreatePendingPayment(ctx, productCfg.AmountCents, productCfg.Currency, productType)
	if err != nil {
		slog.Error("payment create pending row failed",
			"product", string(productType),
			"error", err,
		)
		return nil, fmt.Errorf("create pending payment: %w", err)
	}

	slog.Info("payment pending row created",
		"payment_id", paymentRow.ID,
		"product", string(productType),
		"amount_cents", productCfg.AmountCents,
		"currency", productCfg.Currency,
	)

	successURL := buildFrontendURL(s.settings.FrontendURL, "/pay/success", map[string]string{"session_id": stripeCheckoutSessionPlaceholder, "lang": normalizedLang})
	cancelURL := buildFrontendURL(s.settings.FrontendURL, "/pay", map[string]string{"lang": normalizedLang})

	slog.Info("payment creating stripe checkout session",
		"payment_id", paymentRow.ID,
		"success_url", successURL,
		"cancel_url", cancelURL,
		"frontend_url", s.settings.FrontendURL,
	)

	checkout, err := s.stripe.CreateCheckoutSession(ctx, CreateCheckoutSessionParams{
		AmountCents:       productCfg.AmountCents,
		Currency:          productCfg.Currency,
		ProductName:       productCfg.ProductName,
		SuccessURL:        successURL,
		CancelURL:         cancelURL,
		ClientReferenceID: paymentRow.ID,
	})
	if err != nil {
		slog.Error("payment stripe checkout session create failed",
			"payment_id", paymentRow.ID,
			"error", err,
		)
		_, _ = s.store.MarkPaymentFailed(ctx, PaymentReference{PaymentID: paymentRow.ID}, "STRIPE_CHECKOUT_CREATE_FAILED", truncateFailureDetail(err))
		return nil, err
	}

	slog.Info("payment stripe checkout session created",
		"payment_id", paymentRow.ID,
		"checkout_session_id", checkout.ID,
	)

	if _, err := s.store.AttachCheckoutSessionID(ctx, paymentRow.ID, checkout.ID); err != nil {
		slog.Error("payment attach checkout session id failed",
			"payment_id", paymentRow.ID,
			"checkout_session_id", checkout.ID,
			"error", err,
		)
		_, _ = s.store.MarkPaymentFailed(ctx, PaymentReference{PaymentID: paymentRow.ID, CheckoutSessionID: checkout.ID}, "PAYMENT_LINK_FAILED", truncateFailureDetail(err))
		return nil, fmt.Errorf("attach checkout session id: %w", err)
	}

	return checkout, nil
}

func (s *Service) HandleWebhook(ctx context.Context, payload []byte, signatureHeader string) error {
	event, err := s.stripe.ParseWebhookEvent(payload, signatureHeader)
	if err != nil {
		slog.Error("payment webhook parse failed",
			"error", err,
		)
		return err
	}
	if event == nil || event.Type != "checkout.session.completed" || event.CheckoutSession == nil {
		eventType := ""
		if event != nil {
			eventType = event.Type
		}
		slog.Info("payment webhook ignored event",
			"event_type", eventType,
		)
		return nil
	}

	checkout := event.CheckoutSession
	ref := PaymentReference{
		PaymentID:         strings.TrimSpace(checkout.ClientReferenceID),
		CheckoutSessionID: strings.TrimSpace(checkout.ID),
	}

	slog.Info("payment webhook received checkout.session.completed",
		"payment_id", ref.PaymentID,
		"checkout_session_id", ref.CheckoutSessionID,
		"amount_total", checkout.AmountTotal,
		"currency", checkout.Currency,
		"payment_status", checkout.PaymentStatus,
	)

	paymentRow, err := s.store.GetPayment(ctx, ref)
	if err != nil {
		slog.Error("payment webhook get payment failed",
			"payment_id", ref.PaymentID,
			"checkout_session_id", ref.CheckoutSessionID,
			"error", err,
		)
		return fmt.Errorf("get payment: %w", err)
	}

	if !amountCurrencyMatch(checkout, paymentRow.AmountCents, paymentRow.Currency) {
		slog.Warn("payment webhook amount/currency mismatch",
			"payment_id", ref.PaymentID,
			"checkout_session_id", ref.CheckoutSessionID,
			"stripe_amount", checkout.AmountTotal,
			"stripe_currency", checkout.Currency,
			"stored_amount", paymentRow.AmountCents,
			"stored_currency", paymentRow.Currency,
		)
		if _, markErr := s.store.MarkPaymentFailed(ctx, ref, "PAYMENT_AMOUNT_MISMATCH", "Stripe checkout amount or currency did not match stored payment values"); markErr != nil {
			return fmt.Errorf("mark payment failed after amount mismatch: %w", markErr)
		}
		return nil
	}

	paymentRow, err = s.store.MarkPaymentPaid(ctx, ref, s.nowFn().UTC())
	if err != nil {
		slog.Error("payment webhook mark paid failed",
			"payment_id", ref.PaymentID,
			"checkout_session_id", ref.CheckoutSessionID,
			"error", err,
		)
		return fmt.Errorf("mark payment paid: %w", err)
	}
	if paymentRow.Status != StatusPaidUnprovisioned {
		slog.Info("payment webhook skipping provision, status already advanced",
			"payment_id", ref.PaymentID,
			"status", string(paymentRow.Status),
		)
		return nil
	}

	if _, err := s.store.ProvisionIfNeeded(ctx, checkout.ID, s.nowFn().UTC(), s.buildFulfillmentData); err != nil {
		slog.Error("payment webhook provision failed",
			"checkout_session_id", checkout.ID,
			"error", err,
		)
		_, _ = s.store.MarkProvisionFailure(ctx, checkout.ID, "PAYMENT_PROVISION_FAILED", truncateFailureDetail(err))
		return fmt.Errorf("provision checkout session: %w", err)
	}

	slog.Info("payment webhook provisioned successfully",
		"payment_id", ref.PaymentID,
		"checkout_session_id", ref.CheckoutSessionID,
	)
	return nil
}

func (s *Service) GetCheckoutStatus(ctx context.Context, checkoutSessionID string) (*CheckoutStatus, error) {
	result, err := s.store.ResolveCheckoutSessionForPoll(ctx, strings.TrimSpace(checkoutSessionID), s.nowFn().UTC(), s.buildFulfillmentData)
	if err != nil {
		switch {
		case errors.Is(err, shared.ErrNotFound), errors.Is(err, ErrRevealExpired), errors.Is(err, ErrRevealConsumed):
			return nil, err
		default:
			_, _ = s.store.MarkProvisionFailure(ctx, checkoutSessionID, "PAYMENT_PROVISION_FAILED", truncateFailureDetail(err))
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
		switch result.Payment.ProductType {
		case ProductTypeDirectSession:
			return &CheckoutStatus{
				Status:      "ready",
				ProductType: ProductTypeDirectSession,
				SessionCode: result.SessionCode,
				PIN:         result.PIN,
			}, nil
		case ProductTypeCouponPack10:
			return &CheckoutStatus{
				Status:            "ready",
				ProductType:       ProductTypeCouponPack10,
				CouponCode:        result.CouponCode,
				CouponMaxUses:     result.CouponMaxUses,
				CouponCurrentUses: result.CouponCurrentUses,
			}, nil
		default:
			return nil, fmt.Errorf("%w: %q", ErrUnknownProductType, result.Payment.ProductType)
		}
	default:
		return nil, fmt.Errorf("unsupported payment status %q", result.Payment.Status)
	}
}

func (s *Service) buildFulfillmentData(productType ProductType, paymentID string) (*FulfillmentData, error) {
	switch productType {
	case ProductTypeDirectSession:
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
		return &FulfillmentData{
			ProductType:            ProductTypeDirectSession,
			SessionCode:            sessionCode,
			PIN:                    pin,
			PINHash:                string(pinHash),
			PaymentID:              paymentID,
			ExpiresAt:              now.Add(time.Duration(s.settings.SessionExpiryHours) * time.Hour),
			RevealExpiresAt:        now.Add(s.settings.RevealPINTTL),
			InterviewBudgetSeconds: s.settings.InterviewBudgetSeconds,
		}, nil
	case ProductTypeCouponPack10:
		code, err := generateCouponCode(couponPack10Prefix, couponTokenLength)
		if err != nil {
			return nil, fmt.Errorf("generate coupon code: %w", err)
		}
		return &FulfillmentData{
			ProductType:       ProductTypeCouponPack10,
			PaymentID:         paymentID,
			CouponCode:        code,
			CouponMaxUses:     couponPack10MaxUses,
			CouponCurrentUses: 0,
			CouponSource:      fmt.Sprintf("payment:%s", paymentID),
		}, nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownProductType, productType)
	}
}

func (s *Service) productConfig(productType ProductType) (ProductConfig, error) {
	switch productType {
	case ProductTypeDirectSession:
		return s.settings.DirectSession, nil
	case ProductTypeCouponPack10:
		return s.settings.CouponPack10, nil
	default:
		return ProductConfig{}, fmt.Errorf("%w: %q", ErrUnknownProductType, productType)
	}
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

func generateCouponCode(prefix string, tokenLength int) (string, error) {
	if tokenLength < 4 {
		return "", fmt.Errorf("token length must be at least 4")
	}

	token, err := randomToken(tokenLength)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%s", prefix, token), nil
}

func randomToken(length int) (string, error) {
	var b strings.Builder
	b.Grow(length)

	max := big.NewInt(int64(len(couponAlphabet)))
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b.WriteByte(couponAlphabet[n.Int64()])
	}
	return b.String(), nil
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
