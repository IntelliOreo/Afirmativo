package payment

import (
	"net/http"

	"github.com/afirmativo/backend/internal/shared"
)

// Handler exposes payment endpoints.
type Handler struct{}

// NewHandler creates a payment handler.
func NewHandler() *Handler {
	return &Handler{}
}

// HandleCheckout returns a stable not-implemented response until Stripe integration is added.
func (h *Handler) HandleCheckout(w http.ResponseWriter, _ *http.Request) {
	shared.WriteJSON(w, http.StatusNotImplemented, shared.ErrorResponse{
		Error: "Card checkout is not enabled in this environment",
		Code:  "PAYMENT_NOT_IMPLEMENTED",
	})
}

// HandleWebhook returns a stable not-implemented response until Stripe integration is added.
func (h *Handler) HandleWebhook(w http.ResponseWriter, _ *http.Request) {
	shared.WriteJSON(w, http.StatusNotImplemented, shared.ErrorResponse{
		Error: "Stripe webhook is not enabled in this environment",
		Code:  "PAYMENT_NOT_IMPLEMENTED",
	})
}
