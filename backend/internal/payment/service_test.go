package payment

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

type fakeStore struct {
	createPendingPaymentFn    func(context.Context, int, string, ProductType) (*Payment, error)
	attachCheckoutSessionIDFn func(context.Context, string, string) (*Payment, error)
	getPaymentFn              func(context.Context, PaymentReference) (*Payment, error)
	markPaymentFailedFn       func(context.Context, PaymentReference, string, string) (*Payment, error)
	markPaymentPaidFn         func(context.Context, PaymentReference, time.Time) (*Payment, error)
	provisionIfNeededFn       func(context.Context, string, time.Time, BuildFulfillmentFunc) (*Payment, error)
	resolveCheckoutForPollFn  func(context.Context, string, time.Time, BuildFulfillmentFunc) (*PollResult, error)
	markProvisionFailureFn    func(context.Context, string, string, string) (*Payment, error)
}

func (f *fakeStore) CreatePendingPayment(ctx context.Context, amountCents int, currency string, productType ProductType) (*Payment, error) {
	return f.createPendingPaymentFn(ctx, amountCents, currency, productType)
}

func (f *fakeStore) AttachCheckoutSessionID(ctx context.Context, paymentID, checkoutSessionID string) (*Payment, error) {
	return f.attachCheckoutSessionIDFn(ctx, paymentID, checkoutSessionID)
}

func (f *fakeStore) GetPayment(ctx context.Context, ref PaymentReference) (*Payment, error) {
	return f.getPaymentFn(ctx, ref)
}

func (f *fakeStore) MarkPaymentFailed(ctx context.Context, ref PaymentReference, failureCode, failureDetail string) (*Payment, error) {
	return f.markPaymentFailedFn(ctx, ref, failureCode, failureDetail)
}

func (f *fakeStore) MarkPaymentPaid(ctx context.Context, ref PaymentReference, now time.Time) (*Payment, error) {
	return f.markPaymentPaidFn(ctx, ref, now)
}

func (f *fakeStore) ProvisionIfNeeded(ctx context.Context, checkoutSessionID string, now time.Time, buildFulfillment BuildFulfillmentFunc) (*Payment, error) {
	return f.provisionIfNeededFn(ctx, checkoutSessionID, now, buildFulfillment)
}

func (f *fakeStore) ResolveCheckoutSessionForPoll(ctx context.Context, checkoutSessionID string, now time.Time, buildFulfillment BuildFulfillmentFunc) (*PollResult, error) {
	return f.resolveCheckoutForPollFn(ctx, checkoutSessionID, now, buildFulfillment)
}

func (f *fakeStore) MarkProvisionFailure(ctx context.Context, checkoutSessionID, failureCode, failureDetail string) (*Payment, error) {
	return f.markProvisionFailureFn(ctx, checkoutSessionID, failureCode, failureDetail)
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestServiceCreateCheckout_PassesLanguageIntoStripeURLs(t *testing.T) {
	t.Parallel()

	var capturedSuccessURL string
	var capturedCancelURL string
	var capturedClientReferenceID string
	var capturedPaymentMethodType string
	var capturedProductName string

	httpClient := &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body error = %v", err)
		}
		values, err := url.ParseQuery(string(body))
		if err != nil {
			t.Fatalf("ParseQuery() error = %v", err)
		}
		capturedSuccessURL = values.Get("success_url")
		capturedCancelURL = values.Get("cancel_url")
		capturedClientReferenceID = values.Get("client_reference_id")
		capturedPaymentMethodType = values.Get("payment_method_types[0]")
		capturedProductName = values.Get("line_items[0][price_data][product_data][name]")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"cs_test_123","url":"https://checkout.stripe.com/c/pay/cs_test_123"}`)),
		}, nil
	})}

	store := &fakeStore{
		createPendingPaymentFn: func(_ context.Context, amountCents int, currency string, productType ProductType) (*Payment, error) {
			if amountCents != 499 || currency != "usd" || productType != ProductTypeDirectSession {
				t.Fatalf("CreatePendingPayment() got (%d, %q, %q), want (499, \"usd\", %q)", amountCents, currency, productType, ProductTypeDirectSession)
			}
			return &Payment{ID: "11111111-1111-1111-1111-111111111111", ProductType: productType}, nil
		},
		attachCheckoutSessionIDFn: func(_ context.Context, paymentID, checkoutSessionID string) (*Payment, error) {
			if paymentID != "11111111-1111-1111-1111-111111111111" {
				t.Fatalf("paymentID = %q", paymentID)
			}
			if checkoutSessionID != "cs_test_123" {
				t.Fatalf("checkoutSessionID = %q", checkoutSessionID)
			}
			return &Payment{ID: paymentID, CheckoutSessionID: checkoutSessionID}, nil
		},
		markPaymentFailedFn: func(_ context.Context, _ PaymentReference, _, _ string) (*Payment, error) {
			t.Fatalf("MarkPaymentFailed() should not be called")
			return nil, nil
		},
	}

	svc := NewService(Deps{
		Store: store,
		Stripe: NewStripeClient(StripeClientConfig{
			SecretKey:     "sk_test_123",
			WebhookSecret: "whsec_test",
			BaseURL:       "https://stripe.test",
			HTTPClient:    httpClient,
		}),
	}, Settings{
		FrontendURL:            "http://localhost:3000",
		SessionExpiryHours:     24,
		InterviewBudgetSeconds: 2400,
		DirectSession: ProductConfig{
			AmountCents: 499,
			Currency:    "usd",
			ProductName: "Afirmativo Session Access",
		},
		CouponPack10: ProductConfig{
			AmountCents: 3500,
			Currency:    "usd",
			ProductName: "Afirmativo 10-Use Coupon Pack",
		},
	})

	checkout, err := svc.CreateCheckout(context.Background(), "en", "")
	if err != nil {
		t.Fatalf("CreateCheckout() error = %v", err)
	}
	if checkout.ID != "cs_test_123" {
		t.Fatalf("checkout.ID = %q", checkout.ID)
	}
	if capturedClientReferenceID != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("client_reference_id = %q", capturedClientReferenceID)
	}
	if capturedPaymentMethodType != "card" {
		t.Fatalf("payment_method_types[0] = %q, want card", capturedPaymentMethodType)
	}
	if capturedProductName != "Afirmativo Session Access" {
		t.Fatalf("product_name = %q", capturedProductName)
	}
	if !strings.Contains(capturedSuccessURL, "lang=en") || !strings.Contains(capturedSuccessURL, "session_id={CHECKOUT_SESSION_ID}") {
		t.Fatalf("success_url = %q, want lang + literal checkout placeholder", capturedSuccessURL)
	}
	if capturedCancelURL != "http://localhost:3000/pay?lang=en" {
		t.Fatalf("cancel_url = %q", capturedCancelURL)
	}
}

func TestServiceCreateCheckout_UsesCouponPackProductConfig(t *testing.T) {
	t.Parallel()

	var capturedProductName string
	var capturedAmount string

	httpClient := &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body error = %v", err)
		}
		values, err := url.ParseQuery(string(body))
		if err != nil {
			t.Fatalf("ParseQuery() error = %v", err)
		}
		capturedProductName = values.Get("line_items[0][price_data][product_data][name]")
		capturedAmount = values.Get("line_items[0][price_data][unit_amount]")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"cs_test_pack","url":"https://checkout.stripe.com/c/pay/cs_test_pack"}`)),
		}, nil
	})}

	store := &fakeStore{
		createPendingPaymentFn: func(_ context.Context, amountCents int, currency string, productType ProductType) (*Payment, error) {
			if amountCents != 3500 || currency != "usd" || productType != ProductTypeCouponPack10 {
				t.Fatalf("CreatePendingPayment() got (%d, %q, %q)", amountCents, currency, productType)
			}
			return &Payment{ID: "11111111-1111-1111-1111-111111111111", ProductType: productType}, nil
		},
		attachCheckoutSessionIDFn: func(_ context.Context, paymentID, checkoutSessionID string) (*Payment, error) {
			return &Payment{ID: paymentID, CheckoutSessionID: checkoutSessionID}, nil
		},
		markPaymentFailedFn: func(_ context.Context, _ PaymentReference, _, _ string) (*Payment, error) {
			t.Fatalf("MarkPaymentFailed() should not be called")
			return nil, nil
		},
	}

	svc := NewService(Deps{
		Store: store,
		Stripe: NewStripeClient(StripeClientConfig{
			SecretKey:     "sk_test_123",
			WebhookSecret: "whsec_test",
			BaseURL:       "https://stripe.test",
			HTTPClient:    httpClient,
		}),
	}, Settings{
		FrontendURL: "http://localhost:3000",
		DirectSession: ProductConfig{
			AmountCents: 499,
			Currency:    "usd",
			ProductName: "Afirmativo Session Access",
		},
		CouponPack10: ProductConfig{
			AmountCents: 3500,
			Currency:    "usd",
			ProductName: "Afirmativo 10-Use Coupon Pack",
		},
	})

	if _, err := svc.CreateCheckout(context.Background(), "en", string(ProductTypeCouponPack10)); err != nil {
		t.Fatalf("CreateCheckout() error = %v", err)
	}
	if capturedProductName != "Afirmativo 10-Use Coupon Pack" {
		t.Fatalf("product_name = %q", capturedProductName)
	}
	if capturedAmount != "3500" {
		t.Fatalf("amount = %q", capturedAmount)
	}
}

func TestServiceHandleWebhook_RejectsInvalidSignature(t *testing.T) {
	t.Parallel()

	store := &fakeStore{}
	svc := NewService(Deps{
		Store: store,
		Stripe: NewStripeClient(StripeClientConfig{
			SecretKey:     "sk_test_123",
			WebhookSecret: "whsec_test",
		}),
	}, Settings{})

	err := svc.HandleWebhook(context.Background(), []byte(`{"type":"checkout.session.completed"}`), "bad")
	if err == nil {
		t.Fatal("HandleWebhook() expected error")
	}
	if err != ErrInvalidStripeSignature {
		t.Fatalf("HandleWebhook() error = %v, want ErrInvalidStripeSignature", err)
	}
}

func TestServiceHandleWebhook_MarksAmountMismatchAsFailed(t *testing.T) {
	t.Parallel()

	var markedFailedCode string
	var markedFailedDetail string
	store := &fakeStore{
		getPaymentFn: func(_ context.Context, ref PaymentReference) (*Payment, error) {
			if ref.PaymentID != "11111111-1111-1111-1111-111111111111" {
				t.Fatalf("ref.PaymentID = %q", ref.PaymentID)
			}
			return &Payment{
				ID:          ref.PaymentID,
				ProductType: ProductTypeDirectSession,
				AmountCents: 499,
				Currency:    "usd",
			}, nil
		},
		markPaymentFailedFn: func(_ context.Context, ref PaymentReference, failureCode, failureDetail string) (*Payment, error) {
			if ref.PaymentID != "11111111-1111-1111-1111-111111111111" {
				t.Fatalf("ref.PaymentID = %q", ref.PaymentID)
			}
			markedFailedCode = failureCode
			markedFailedDetail = failureDetail
			return &Payment{ID: ref.PaymentID, Status: StatusFailed}, nil
		},
	}

	webhookSecret := "whsec_test"
	svc := NewService(Deps{
		Store: store,
		Stripe: NewStripeClient(StripeClientConfig{
			SecretKey:     "sk_test_123",
			WebhookSecret: webhookSecret,
		}),
	}, Settings{
		DirectSession: ProductConfig{
			AmountCents: 499,
			Currency:    "usd",
			ProductName: "Afirmativo Session Access",
		},
		CouponPack10: ProductConfig{
			AmountCents: 3500,
			Currency:    "usd",
			ProductName: "Afirmativo 10-Use Coupon Pack",
		},
	})

	payload := []byte(`{"type":"checkout.session.completed","data":{"object":{"id":"cs_test_123","client_reference_id":"11111111-1111-1111-1111-111111111111","amount_total":7000,"currency":"usd","payment_status":"paid"}}}`)
	if err := svc.HandleWebhook(context.Background(), payload, signedStripeHeader(t, webhookSecret, payload, time.Now().UTC())); err != nil {
		t.Fatalf("HandleWebhook() error = %v", err)
	}
	if markedFailedCode != "PAYMENT_AMOUNT_MISMATCH" {
		t.Fatalf("markedFailedCode = %q", markedFailedCode)
	}
	if !strings.Contains(markedFailedDetail, "did not match") {
		t.Fatalf("markedFailedDetail = %q", markedFailedDetail)
	}
}

func TestServiceGetCheckoutStatus_ProvisionFailureFallsBackToPending(t *testing.T) {
	t.Parallel()

	var markedFailure bool
	store := &fakeStore{
		resolveCheckoutForPollFn: func(_ context.Context, _ string, _ time.Time, _ BuildFulfillmentFunc) (*PollResult, error) {
			return nil, errors.New("db timeout")
		},
		markProvisionFailureFn: func(_ context.Context, checkoutSessionID, failureCode, failureDetail string) (*Payment, error) {
			if checkoutSessionID != "cs_test_123" {
				t.Fatalf("checkoutSessionID = %q", checkoutSessionID)
			}
			if failureCode != "PAYMENT_PROVISION_FAILED" {
				t.Fatalf("failureCode = %q", failureCode)
			}
			if !strings.Contains(failureDetail, "db timeout") {
				t.Fatalf("failureDetail = %q", failureDetail)
			}
			markedFailure = true
			return &Payment{}, nil
		},
	}

	svc := NewService(Deps{Store: store}, Settings{})
	status, err := svc.GetCheckoutStatus(context.Background(), "cs_test_123")
	if err != nil {
		t.Fatalf("GetCheckoutStatus() error = %v", err)
	}
	if status.Status != "pending" {
		t.Fatalf("status = %#v", status)
	}
	if !markedFailure {
		t.Fatal("expected MarkProvisionFailure() to be called")
	}
}

func TestServiceGetCheckoutStatus_ReturnsCouponPackReady(t *testing.T) {
	t.Parallel()

	store := &fakeStore{
		resolveCheckoutForPollFn: func(_ context.Context, _ string, _ time.Time, _ BuildFulfillmentFunc) (*PollResult, error) {
			return &PollResult{
				Payment: &Payment{
					ID:          "11111111-1111-1111-1111-111111111111",
					ProductType: ProductTypeCouponPack10,
					Status:      StatusProvisioned,
				},
				CouponCode:        "PACK10-ABCD2345",
				CouponMaxUses:     10,
				CouponCurrentUses: 0,
			}, nil
		},
	}

	svc := NewService(Deps{Store: store}, Settings{})
	status, err := svc.GetCheckoutStatus(context.Background(), "cs_test_pack")
	if err != nil {
		t.Fatalf("GetCheckoutStatus() error = %v", err)
	}
	if status.Status != "ready" || status.ProductType != ProductTypeCouponPack10 {
		t.Fatalf("status = %#v", status)
	}
	if status.CouponCode != "PACK10-ABCD2345" || status.CouponMaxUses != 10 || status.CouponCurrentUses != 0 {
		t.Fatalf("coupon status = %#v", status)
	}
}

func TestServiceBuildFulfillmentData_UsesFixedCouponPackSize(t *testing.T) {
	t.Parallel()

	svc := NewService(Deps{}, Settings{})

	fulfillment, err := svc.buildFulfillmentData(ProductTypeCouponPack10, "11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatalf("buildFulfillmentData() error = %v", err)
	}
	if fulfillment.CouponMaxUses != couponPack10MaxUses {
		t.Fatalf("CouponMaxUses = %d, want %d", fulfillment.CouponMaxUses, couponPack10MaxUses)
	}
	if fulfillment.CouponCurrentUses != 0 {
		t.Fatalf("CouponCurrentUses = %d, want 0", fulfillment.CouponCurrentUses)
	}
	if !strings.HasPrefix(fulfillment.CouponCode, "PACK10-") {
		t.Fatalf("CouponCode = %q, want PACK10- prefix", fulfillment.CouponCode)
	}
}

func signedStripeHeader(t *testing.T, secret string, payload []byte, timestamp time.Time) string {
	t.Helper()
	signedPayload := fmt.Sprintf("%d.%s", timestamp.Unix(), payload)
	mac := hmac.New(sha256.New, []byte(secret))
	if _, err := mac.Write([]byte(signedPayload)); err != nil {
		t.Fatalf("mac.Write() error = %v", err)
	}
	return fmt.Sprintf("t=%d,v1=%s", timestamp.Unix(), hex.EncodeToString(mac.Sum(nil)))
}
