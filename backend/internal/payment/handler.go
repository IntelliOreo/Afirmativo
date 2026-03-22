package payment

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/afirmativo/backend/internal/shared"
)

const (
	maxJSONBody    = 64 * 1024
	maxWebhookBody = 256 * 1024
)

type Handler struct {
	svc *Service
}

type checkoutRequest struct {
	Lang string `json:"lang"`
}

type checkoutResponse struct {
	URL string `json:"url"`
}

type checkoutStatusResponse struct {
	Status      string `json:"status"`
	SessionCode string `json:"session_code,omitempty"`
	PIN         string `json:"pin,omitempty"`
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) HandleCheckout(w http.ResponseWriter, r *http.Request) {
	var req checkoutRequest
	if err := shared.DecodeJSON(r, &req, maxJSONBody); err != nil {
		shared.WriteError(w, shared.ErrBadRequest, "Invalid request body", "BAD_REQUEST")
		return
	}

	checkout, err := h.svc.CreateCheckout(r.Context(), req.Lang)
	if err != nil {
		if errors.Is(err, shared.ErrBadRequest) {
			shared.WriteError(w, shared.ErrBadRequest, "Language must be en or es", "BAD_REQUEST")
			return
		}
		shared.WriteError(w, shared.ErrInternal, "Could not start checkout", "PAYMENT_CHECKOUT_FAILED")
		return
	}

	shared.WriteJSON(w, http.StatusOK, checkoutResponse{URL: checkout.URL})
}

func (h *Handler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	bodyReader := http.MaxBytesReader(w, r.Body, maxWebhookBody)
	defer bodyReader.Close()

	payload, err := io.ReadAll(bodyReader)
	if err != nil {
		shared.WriteError(w, shared.ErrBadRequest, "Invalid webhook payload", "BAD_REQUEST")
		return
	}

	if err := h.svc.HandleWebhook(r.Context(), payload, r.Header.Get("Stripe-Signature")); err != nil {
		if errors.Is(err, ErrInvalidStripeSignature) {
			shared.WriteErrorStatus(w, http.StatusBadRequest, "Invalid Stripe signature", "INVALID_STRIPE_SIGNATURE")
			return
		}
		shared.WriteError(w, shared.ErrInternal, "Could not process Stripe webhook", "PAYMENT_WEBHOOK_FAILED")
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) HandleCheckoutSessionStatus(w http.ResponseWriter, r *http.Request) {
	checkoutSessionID := strings.TrimSpace(r.PathValue("id"))
	if checkoutSessionID == "" {
		shared.WriteError(w, shared.ErrBadRequest, "Checkout session id is required", "BAD_REQUEST")
		return
	}

	status, err := h.svc.GetCheckoutStatus(r.Context(), checkoutSessionID)
	if err != nil {
		switch {
		case errors.Is(err, shared.ErrNotFound):
			shared.WriteError(w, shared.ErrNotFound, "Checkout session not found", "PAYMENT_NOT_FOUND")
		case errors.Is(err, ErrRevealExpired):
			shared.WriteError(w, shared.ErrGone, "Payment reveal expired", "PAYMENT_REVEAL_EXPIRED")
		case errors.Is(err, ErrRevealConsumed):
			shared.WriteError(w, shared.ErrConflict, "Payment reveal already consumed", "PAYMENT_REVEAL_CONSUMED")
		default:
			shared.WriteError(w, shared.ErrInternal, "Could not resolve checkout session", "PAYMENT_STATUS_FAILED")
		}
		return
	}

	switch status.Status {
	case "pending":
		shared.WriteJSON(w, http.StatusAccepted, checkoutStatusResponse{Status: "pending"})
	case "failed":
		shared.WriteErrorStatus(w, http.StatusConflict, "Payment could not be completed", firstNonEmpty(status.FailureCode, "PAYMENT_FAILED"))
	case "ready":
		shared.WriteJSON(w, http.StatusOK, checkoutStatusResponse{
			Status:      "ready",
			SessionCode: status.SessionCode,
			PIN:         status.PIN,
		})
	default:
		shared.WriteError(w, shared.ErrInternal, "Unsupported checkout session status", "PAYMENT_STATUS_FAILED")
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
