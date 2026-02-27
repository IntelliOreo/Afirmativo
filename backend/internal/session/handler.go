// HTTP handlers for session endpoints:
//
//	POST /api/coupon/validate  — HandleValidateCoupon
//	POST /api/session/verify   — HandleVerifySession
package session

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/afirmativo/backend/internal/shared"
)

// Handler holds session HTTP handlers.
type Handler struct {
	svc *Service
}

// NewHandler creates a Handler backed by the given Service.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// validateRequest is the JSON body for POST /api/coupon/validate.
type validateRequest struct {
	Code string `json:"code"`
}

// validateResponse is the success response for POST /api/coupon/validate.
type validateResponse struct {
	Valid       bool   `json:"valid"`
	SessionCode string `json:"session_code"`
	PIN         string `json:"pin"`
}

// validateErrorResponse is the error response for POST /api/coupon/validate.
type validateErrorResponse struct {
	Valid bool   `json:"valid"`
	Error string `json:"error"`
	Code  string `json:"code"`
}

const maxJSONBody = 64 * 1024 // 64KB default for JSON endpoints

// HandleValidateCoupon handles POST /api/coupon/validate.
func (h *Handler) HandleValidateCoupon(w http.ResponseWriter, r *http.Request) {
	var req validateRequest
	if err := shared.DecodeJSON(r, &req, maxJSONBody); err != nil {
		shared.WriteError(w, shared.ErrBadRequest, "Invalid request body", "BAD_REQUEST")
		return
	}

	if req.Code == "" {
		shared.WriteError(w, shared.ErrBadRequest, "Coupon code is required", "BAD_REQUEST")
		return
	}

	result, err := h.svc.ValidateCoupon(r.Context(), req.Code)
	if err != nil {
		if errors.Is(err, shared.ErrCouponInvalid) {
			shared.WriteJSON(w, http.StatusBadRequest, validateErrorResponse{
				Valid: false,
				Error: "Coupon invalid or already used",
				Code:  "COUPON_INVALID",
			})
			return
		}
		slog.Error("coupon validation failed", "error", err)
		shared.WriteError(w, shared.ErrInternal, "Internal server error", "INTERNAL_ERROR")
		return
	}

	shared.WriteJSON(w, http.StatusOK, validateResponse{
		Valid:       true,
		SessionCode: result.SessionCode,
		PIN:         result.PIN,
	})
}

// verifyRequest is the JSON body for POST /api/session/verify.
type verifyRequest struct {
	SessionCode string `json:"sessionCode"`
	PIN         string `json:"pin"`
}

// HandleVerifySession handles POST /api/session/verify.
func (h *Handler) HandleVerifySession(w http.ResponseWriter, r *http.Request) {
	var req verifyRequest
	if err := shared.DecodeJSON(r, &req, maxJSONBody); err != nil {
		shared.WriteError(w, shared.ErrBadRequest, "Invalid request body", "BAD_REQUEST")
		return
	}

	if req.SessionCode == "" || req.PIN == "" {
		shared.WriteError(w, shared.ErrBadRequest, "Session code and PIN are required", "BAD_REQUEST")
		return
	}

	sess, err := h.svc.VerifySession(r.Context(), req.SessionCode, req.PIN)
	if err != nil {
		switch {
		case errors.Is(err, shared.ErrNotFound):
			shared.WriteError(w, shared.ErrNotFound, "Session not found", "SESSION_NOT_FOUND")
		case errors.Is(err, ErrPINIncorrect):
			shared.WriteError(w, shared.ErrUnauthorized, "PIN incorrect", "PIN_INCORRECT")
		case errors.Is(err, ErrSessionExpired):
			shared.WriteError(w, shared.ErrGone, "Session expired", "SESSION_EXPIRED")
		default:
			slog.Error("session verify failed", "error", err)
			shared.WriteError(w, shared.ErrInternal, "Internal server error", "INTERNAL_ERROR")
		}
		return
	}

	shared.WriteJSON(w, http.StatusOK, map[string]any{
		"session": map[string]any{
			"session_code":  sess.SessionCode,
			"status":        sess.Status,
			"track":         sess.Track,
			"timer_seconds": sess.TimerSeconds,
			"created_at":    sess.CreatedAt,
			"expires_at":    sess.ExpiresAt,
			"started_at":    sess.StartedAt,
		},
	})
}
