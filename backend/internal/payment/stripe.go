package payment

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultStripeBaseURL    = "https://api.stripe.com"
	defaultWebhookTolerance = 5 * time.Minute
)

type StripeClientConfig struct {
	SecretKey        string
	WebhookSecret    string
	BaseURL          string
	HTTPClient       *http.Client
	WebhookNowFn     func() time.Time
	WebhookTolerance time.Duration
}

type StripeClient struct {
	secretKey        string
	webhookSecret    string
	baseURL          string
	httpClient       *http.Client
	webhookNowFn     func() time.Time
	webhookTolerance time.Duration
}

type CreateCheckoutSessionParams struct {
	AmountCents       int
	Currency          string
	ProductName       string
	SuccessURL        string
	CancelURL         string
	ClientReferenceID string
}

type CreatedCheckoutSession struct {
	ID  string
	URL string
}

type WebhookEvent struct {
	Type             string
	CheckoutSession  *StripeCheckoutSession
}

type StripeCheckoutSession struct {
	ID                string `json:"id"`
	ClientReferenceID string `json:"client_reference_id"`
	AmountTotal       int    `json:"amount_total"`
	Currency          string `json:"currency"`
	PaymentStatus     string `json:"payment_status"`
}

func NewStripeClient(cfg StripeClientConfig) *StripeClient {
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultStripeBaseURL
	}
	nowFn := cfg.WebhookNowFn
	if nowFn == nil {
		nowFn = time.Now
	}
	tolerance := cfg.WebhookTolerance
	if tolerance <= 0 {
		tolerance = defaultWebhookTolerance
	}
	return &StripeClient{
		secretKey:        cfg.SecretKey,
		webhookSecret:    cfg.WebhookSecret,
		baseURL:          baseURL,
		httpClient:       httpClient,
		webhookNowFn:     nowFn,
		webhookTolerance: tolerance,
	}
}

func (c *StripeClient) CreateCheckoutSession(ctx context.Context, params CreateCheckoutSessionParams) (*CreatedCheckoutSession, error) {
	form := url.Values{}
	form.Set("mode", "payment")
	form.Set("payment_method_types[0]", "card")
	form.Set("success_url", params.SuccessURL)
	form.Set("cancel_url", params.CancelURL)
	form.Set("client_reference_id", params.ClientReferenceID)
	form.Set("line_items[0][price_data][currency]", strings.ToLower(params.Currency))
	form.Set("line_items[0][price_data][product_data][name]", params.ProductName)
	form.Set("line_items[0][price_data][unit_amount]", strconv.Itoa(params.AmountCents))
	form.Set("line_items[0][quantity]", "1")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/checkout/sessions", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create stripe checkout request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.secretKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	slog.Info("stripe checkout session request",
		"url", c.baseURL+"/v1/checkout/sessions",
		"client_reference_id", params.ClientReferenceID,
		"amount_cents", params.AmountCents,
		"currency", params.Currency,
	)

	res, err := c.httpClient.Do(req)
	if err != nil {
		slog.Error("stripe checkout session http failed",
			"error", err,
		)
		return nil, fmt.Errorf("send stripe checkout request: %w", err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("read stripe checkout response: %w", err)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		slog.Error("stripe checkout session api error",
			"status", res.StatusCode,
			"body", strings.TrimSpace(string(body)),
		)
		return nil, fmt.Errorf("stripe checkout create failed: status=%d body=%s", res.StatusCode, strings.TrimSpace(string(body)))
	}

	var response struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode stripe checkout response: %w", err)
	}
	if strings.TrimSpace(response.ID) == "" || strings.TrimSpace(response.URL) == "" {
		return nil, fmt.Errorf("stripe checkout response missing id or url")
	}

	slog.Info("stripe checkout session created",
		"checkout_session_id", response.ID,
	)

	return &CreatedCheckoutSession{
		ID:  response.ID,
		URL: response.URL,
	}, nil
}

func (c *StripeClient) ParseWebhookEvent(payload []byte, signatureHeader string) (*WebhookEvent, error) {
	if err := verifyStripeSignature(payload, signatureHeader, c.webhookSecret, c.webhookNowFn().UTC(), c.webhookTolerance); err != nil {
		return nil, err
	}

	var event struct {
		Type string `json:"type"`
		Data struct {
			Object StripeCheckoutSession `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, fmt.Errorf("decode stripe webhook event: %w", err)
	}

	return &WebhookEvent{
		Type:            event.Type,
		CheckoutSession: &event.Data.Object,
	}, nil
}

func verifyStripeSignature(payload []byte, header, secret string, now time.Time, tolerance time.Duration) error {
	if strings.TrimSpace(header) == "" || strings.TrimSpace(secret) == "" {
		return ErrInvalidStripeSignature
	}

	var timestamp int64
	var signatures []string
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "t=") {
			parsed, err := strconv.ParseInt(strings.TrimPrefix(part, "t="), 10, 64)
			if err != nil {
				return ErrInvalidStripeSignature
			}
			timestamp = parsed
			continue
		}
		if strings.HasPrefix(part, "v1=") {
			signatures = append(signatures, strings.TrimPrefix(part, "v1="))
		}
	}
	if timestamp == 0 || len(signatures) == 0 {
		return ErrInvalidStripeSignature
	}

	signedPayload := strconv.FormatInt(timestamp, 10) + "." + string(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	if _, err := mac.Write([]byte(signedPayload)); err != nil {
		return fmt.Errorf("hash stripe webhook payload: %w", err)
	}
	expected := hex.EncodeToString(mac.Sum(nil))

	valid := false
	for _, signature := range signatures {
		if hmac.Equal([]byte(signature), []byte(expected)) {
			valid = true
			break
		}
	}
	if !valid {
		return ErrInvalidStripeSignature
	}

	signedAt := time.Unix(timestamp, 0).UTC()
	if signedAt.Before(now.Add(-tolerance)) || signedAt.After(now.Add(tolerance)) {
		return ErrInvalidStripeSignature
	}
	return nil
}
