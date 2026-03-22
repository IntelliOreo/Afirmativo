package voice

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/afirmativo/backend/internal/shared"
)

const (
	maxTokenBodyBytes      = 8 * 1024
	maxTokenTTLSeconds     = 3600
	defaultTokenTTLSeconds = 30
)

type mintTokenRequest struct {
	SessionCode string `json:"session_code"`
	TTLSeconds  int    `json:"ttl_seconds"`
}

type mintTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	Provider    string `json:"provider"`
	Model       string `json:"model"`
}

// Handler exposes voice token endpoints.
type Handler struct {
	client            *Client
	defaultTTLSeconds int
}

// NewHandler creates a voice handler.
func NewHandler(client *Client, defaultTTLSeconds int) *Handler {
	if defaultTTLSeconds <= 0 {
		defaultTTLSeconds = defaultTokenTTLSeconds
	}
	return &Handler{
		client:            client,
		defaultTTLSeconds: defaultTTLSeconds,
	}
}

// HandleMintToken issues a short-lived voice provider token for an authenticated session.
func (h *Handler) HandleMintToken(w http.ResponseWriter, r *http.Request) {
	if h.client == nil {
		slog.Error("voice token handler misconfigured: client missing")
		shared.WriteError(w, shared.ErrInternal, "Voice token service is not configured", "VOICE_NOT_CONFIGURED")
		return
	}

	claims, ok := shared.SessionAuthClaimsFromContext(r.Context())
	if !ok {
		slog.Warn("voice token request rejected: missing auth claims")
		shared.WriteError(w, shared.ErrUnauthorized, "Unauthorized", "UNAUTHORIZED")
		return
	}

	var req mintTokenRequest
	if r.ContentLength > 0 {
		if err := shared.DecodeJSON(r, &req, maxTokenBodyBytes); err != nil {
			slog.Warn("voice token request rejected: invalid body",
				"session_code", claims.SessionCode,
				"error", err,
			)
			shared.WriteError(w, shared.ErrBadRequest, "Invalid request body", "BAD_REQUEST")
			return
		}
	}
	slog.Debug("voice/token payload", "body", req)

	ttlSeconds, validTTL := normalizeTTL(req.TTLSeconds, h.defaultTTLSeconds)
	if !validTTL {
		slog.Warn("voice token request rejected: invalid ttl",
			"session_code", claims.SessionCode,
			"requested_ttl_seconds", req.TTLSeconds,
			"default_ttl_seconds", h.defaultTTLSeconds,
		)
		shared.WriteError(w, shared.ErrBadRequest, "TTL seconds must be between 1 and 3600", "BAD_REQUEST")
		return
	}

	requestSessionCode := strings.TrimSpace(req.SessionCode)
	if requestSessionCode != "" && requestSessionCode != strings.TrimSpace(claims.SessionCode) {
		slog.Warn("voice token request rejected: session mismatch",
			"auth_session_code", claims.SessionCode,
			"request_session_code", requestSessionCode,
		)
		shared.WriteError(w, shared.ErrUnauthorized, "Unauthorized", "SESSION_MISMATCH")
		return
	}
	slog.Debug("voice token mint request",
		"session_code", claims.SessionCode,
		"requested_ttl_seconds", req.TTLSeconds,
		"effective_ttl_seconds", ttlSeconds,
		"provider", h.client.Provider(),
	)

	token, err := h.client.MintToken(r.Context(), ttlSeconds)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusTooManyRequests {
			slog.Warn("voice token request rate limited by provider",
				"session_code", claims.SessionCode,
				"ttl_seconds", ttlSeconds,
				"provider_status", apiErr.StatusCode,
			)
			shared.WriteError(w, shared.ErrRateLimited, "Voice token rate limited", "RATE_LIMITED")
			return
		}
		slog.Error("voice token grant failed",
			"session_code", claims.SessionCode,
			"ttl_seconds", ttlSeconds,
			"error", err,
		)
		shared.WriteError(w, shared.ErrInternal, "Failed to mint voice token", "VOICE_TOKEN_GRANT_FAILED")
		return
	}

	slog.Info("voice token minted",
		"session_code", claims.SessionCode,
		"expires_in", token.ExpiresIn,
		"provider", h.client.Provider(),
	)

	shared.WriteJSON(w, http.StatusOK, mintTokenResponse{
		AccessToken: token.AccessToken,
		TokenType:   token.TokenType,
		ExpiresIn:   token.ExpiresIn,
		Provider:    h.client.Provider(),
		Model:       h.client.Model(),
	})
}

func normalizeTTL(requested, fallback int) (int, bool) {
	if requested == 0 {
		requested = fallback
	}
	if requested <= 0 || requested > maxTokenTTLSeconds {
		return 0, false
	}
	return requested, true
}
