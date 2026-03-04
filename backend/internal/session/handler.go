// HTTP handlers for session endpoints:
//
//	POST /api/coupon/validate  — HandleValidateCoupon
//	POST /api/session/verify   — HandleVerifySession
package session

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/afirmativo/backend/internal/shared"
)

// Handler holds session HTTP handlers.
type Handler struct {
	svc        *Service
	auth       *shared.SessionAuthManager
	authMaxTTL time.Duration
}

// NewHandler creates a Handler backed by the given Service.
func NewHandler(svc *Service, auth *shared.SessionAuthManager, authMaxTTL time.Duration) *Handler {
	if authMaxTTL <= 0 {
		authMaxTTL = time.Hour
	}
	return &Handler{
		svc:        svc,
		auth:       auth,
		authMaxTTL: authMaxTTL,
	}
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
		if h.auth != nil {
			h.auth.ClearCookie(w)
			slog.Debug("cleared auth cookie after failed verify", "session_code", req.SessionCode)
		}
		switch {
		case errors.Is(err, shared.ErrNotFound):
			slog.Debug("session verify failed: session not found", "session_code", req.SessionCode)
			shared.WriteError(w, shared.ErrNotFound, "Session not found", "SESSION_NOT_FOUND")
		case errors.Is(err, ErrPINIncorrect):
			slog.Debug("session verify failed: incorrect PIN", "session_code", req.SessionCode)
			shared.WriteError(w, shared.ErrUnauthorized, "PIN incorrect", "PIN_INCORRECT")
		case errors.Is(err, ErrSessionExpired):
			slog.Debug("session verify failed: session expired", "session_code", req.SessionCode)
			shared.WriteError(w, shared.ErrGone, "Session expired", "SESSION_EXPIRED")
		default:
			slog.Error("session verify failed", "error", err)
			shared.WriteError(w, shared.ErrInternal, "Internal server error", "INTERNAL_ERROR")
		}
		return
	}

	if h.auth == nil {
		slog.Error("session auth manager not configured")
		shared.WriteError(w, shared.ErrInternal, "Internal server error", "INTERNAL_ERROR")
		return
	}

	now := time.Now().UTC()
	tokenExpiresAt := sess.ExpiresAt.UTC()
	maxTokenExpiresAt := now.Add(h.authMaxTTL)
	if tokenExpiresAt.After(maxTokenExpiresAt) {
		tokenExpiresAt = maxTokenExpiresAt
	}

	token, err := h.auth.MintToken(sess.SessionCode, tokenExpiresAt)
	if err != nil {
		slog.Error("failed to mint session auth token", "session_code", sess.SessionCode, "error", err)
		shared.WriteError(w, shared.ErrInternal, "Internal server error", "INTERNAL_ERROR")
		return
	}
	h.auth.SetCookie(w, token, tokenExpiresAt)
	slog.Debug("session verified and auth cookie issued",
		"session_code", sess.SessionCode,
		"token_expires_at", tokenExpiresAt,
		"session_expires_at", sess.ExpiresAt.UTC(),
		"auth_ttl_seconds", int(tokenExpiresAt.Sub(now).Seconds()),
	)

	shared.WriteJSON(w, http.StatusOK, map[string]any{
		"session": map[string]any{
			"session_code":             sess.SessionCode,
			"status":                   sess.Status,
			"track":                    sess.Track,
			"interview_budget_seconds": sess.InterviewBudgetSeconds,
			"interview_lapsed_seconds": sess.InterviewLapsedSeconds,
			"interview_started_at":     sess.InterviewStartedAt,
			"created_at":               sess.CreatedAt,
			"expires_at":               sess.ExpiresAt,
		},
	})
}
