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
	createPendingPaymentFn    func(context.Context, int, string) (*Payment, error)
	attachCheckoutSessionIDFn func(context.Context, string, string) (*Payment, error)
	markPaymentFailedFn       func(context.Context, PaymentReference, string, string) (*Payment, error)
	markPaymentPaidFn         func(context.Context, PaymentReference, time.Time) (*Payment, error)
	provisionIfNeededFn       func(context.Context, string, time.Time, func() (*ProvisionData, error)) (*Payment, error)
	resolveCheckoutForPollFn  func(context.Context, string, time.Time, func() (*ProvisionData, error)) (*PollResult, error)
	markProvisionFailureFn    func(context.Context, string, string, string) (*Payment, error)
}

func (f *fakeStore) CreatePendingPayment(ctx context.Context, amountCents int, currency string) (*Payment, error) {
	return f.createPendingPaymentFn(ctx, amountCents, currency)
}

func (f *fakeStore) AttachCheckoutSessionID(ctx context.Context, paymentID, checkoutSessionID string) (*Payment, error) {
	return f.attachCheckoutSessionIDFn(ctx, paymentID, checkoutSessionID)
}

func (f *fakeStore) MarkPaymentFailed(ctx context.Context, ref PaymentReference, failureCode, failureDetail string) (*Payment, error) {
	return f.markPaymentFailedFn(ctx, ref, failureCode, failureDetail)
}

func (f *fakeStore) MarkPaymentPaid(ctx context.Context, ref PaymentReference, now time.Time) (*Payment, error) {
	return f.markPaymentPaidFn(ctx, ref, now)
}

func (f *fakeStore) ProvisionIfNeeded(ctx context.Context, checkoutSessionID string, now time.Time, buildProvision func() (*ProvisionData, error)) (*Payment, error) {
	return f.provisionIfNeededFn(ctx, checkoutSessionID, now, buildProvision)
}

func (f *fakeStore) ResolveCheckoutSessionForPoll(ctx context.Context, checkoutSessionID string, now time.Time, buildProvision func() (*ProvisionData, error)) (*PollResult, error) {
	return f.resolveCheckoutForPollFn(ctx, checkoutSessionID, now, buildProvision)
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
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"cs_test_123","url":"https://checkout.stripe.com/c/pay/cs_test_123"}`)),
		}, nil
	})}

	store := &fakeStore{
		createPendingPaymentFn: func(_ context.Context, amountCents int, currency string) (*Payment, error) {
			if amountCents != 5000 || currency != "usd" {
				t.Fatalf("CreatePendingPayment() got (%d, %q), want (5000, \"usd\")", amountCents, currency)
			}
			return &Payment{ID: "11111111-1111-1111-1111-111111111111"}, nil
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
		AmountCents:            5000,
		Currency:               "usd",
	})

	checkout, err := svc.CreateCheckout(context.Background(), "en")
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
	if !strings.Contains(capturedSuccessURL, "lang=en") || !strings.Contains(capturedSuccessURL, "session_id={CHECKOUT_SESSION_ID}") {
		t.Fatalf("success_url = %q, want lang + literal checkout placeholder", capturedSuccessURL)
	}
	if capturedCancelURL != "http://localhost:3000/pay?lang=en" {
		t.Fatalf("cancel_url = %q", capturedCancelURL)
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
		AmountCents: 5000,
		Currency:    "usd",
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
		resolveCheckoutForPollFn: func(_ context.Context, _ string, _ time.Time, _ func() (*ProvisionData, error)) (*PollResult, error) {
			return nil, errors.New("db timeout")
		},
		markProvisionFailureFn: func(_ context.Context, checkoutSessionID, failureCode, failureDetail string) (*Payment, error) {
			if checkoutSessionID != "cs_test_123" {
				t.Fatalf("checkoutSessionID = %q", checkoutSessionID)
			}
			if failureCode != "SESSION_PROVISION_FAILED" {
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

func signedStripeHeader(t *testing.T, secret string, payload []byte, timestamp time.Time) string {
	t.Helper()
	signedPayload := fmt.Sprintf("%d.%s", timestamp.Unix(), payload)
	mac := hmac.New(sha256.New, []byte(secret))
	if _, err := mac.Write([]byte(signedPayload)); err != nil {
		t.Fatalf("mac.Write() error = %v", err)
	}
	return fmt.Sprintf("t=%d,v1=%s", timestamp.Unix(), hex.EncodeToString(mac.Sum(nil)))
}
