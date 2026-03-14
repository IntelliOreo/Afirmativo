package shared

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	defaultSessionAuthCookieName = "afirmativo_auth"
	defaultSessionAuthIssuer     = "afirmativo-backend"
	defaultSessionAuthAudience   = "afirmativo-frontend"
	sessionAuthClockSkew         = 30 * time.Second
)

// SessionAuthClaims defines the JWT payload for authenticated session routes.
type SessionAuthClaims struct {
	SessionCode string `json:"session_code"`
	jwt.RegisteredClaims
}

// SessionAuthConfig defines JWT/cookie settings for session auth.
type SessionAuthConfig struct {
	Secret       string
	CookieName   string
	Issuer       string
	Audience     string
	CookieSecure bool
}

// SessionAuthManager handles JWT mint/verify and auth cookie operations.
type SessionAuthManager struct {
	secret       []byte
	cookieName   string
	issuer       string
	audience     string
	cookieSecure bool
	nowFn        func() time.Time
}

// NewSessionAuthManager builds a manager for JWT session authentication.
func NewSessionAuthManager(cfg SessionAuthConfig) (*SessionAuthManager, error) {
	secret := strings.TrimSpace(cfg.Secret)
	if secret == "" {
		return nil, fmt.Errorf("session auth secret is required")
	}
	if len(secret) < 32 {
		return nil, fmt.Errorf("session auth secret must be at least 32 chars")
	}

	cookieName := strings.TrimSpace(cfg.CookieName)
	if cookieName == "" {
		cookieName = defaultSessionAuthCookieName
	}
	issuer := strings.TrimSpace(cfg.Issuer)
	if issuer == "" {
		issuer = defaultSessionAuthIssuer
	}
	audience := strings.TrimSpace(cfg.Audience)
	if audience == "" {
		audience = defaultSessionAuthAudience
	}

	return &SessionAuthManager{
		secret:       []byte(secret),
		cookieName:   cookieName,
		issuer:       issuer,
		audience:     audience,
		cookieSecure: cfg.CookieSecure,
		nowFn:        time.Now,
	}, nil
}

// CookieName returns the configured auth cookie name.
func (m *SessionAuthManager) CookieName() string {
	return m.cookieName
}

// MintToken creates a signed HS256 JWT for a session code.
func (m *SessionAuthManager) MintToken(sessionCode string, expiresAt time.Time) (string, error) {
	code := strings.TrimSpace(sessionCode)
	if code == "" {
		return "", fmt.Errorf("session code is required")
	}

	now := m.nowFn().UTC()
	exp := expiresAt.UTC()
	if !exp.After(now) {
		return "", fmt.Errorf("token expiry must be in the future")
	}

	claims := SessionAuthClaims{
		SessionCode: code,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Audience:  jwt.ClaimStrings{m.audience},
			ExpiresAt: jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(m.secret)
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return signed, nil
}

// ValidateToken verifies token signature and required claims.
func (m *SessionAuthManager) ValidateToken(tokenString string) (*SessionAuthClaims, error) {
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{"HS256"}),
		jwt.WithLeeway(sessionAuthClockSkew),
		jwt.WithIssuer(m.issuer),
		jwt.WithAudience(m.audience),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
	)

	token, err := parser.ParseWithClaims(tokenString, &SessionAuthClaims{}, func(t *jwt.Token) (any, error) {
		return m.secret, nil
	})
	if err != nil {
		return nil, ErrUnauthorized
	}

	claims, ok := token.Claims.(*SessionAuthClaims)
	if !ok || !token.Valid {
		return nil, ErrUnauthorized
	}

	if strings.TrimSpace(claims.SessionCode) == "" {
		return nil, ErrUnauthorized
	}

	return claims, nil
}

// SetCookie writes the signed JWT as an HttpOnly auth cookie.
func (m *SessionAuthManager) SetCookie(w http.ResponseWriter, token string, expiresAt time.Time) {
	maxAge := int(time.Until(expiresAt).Seconds())
	if maxAge < 0 {
		maxAge = 0
	}
	http.SetCookie(w, &http.Cookie{
		Name:     m.cookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt.UTC(),
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   m.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearCookie removes the auth cookie.
func (m *SessionAuthManager) ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     m.cookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0).UTC(),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   m.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClaimsFromRequest validates the JWT stored in the auth cookie.
func (m *SessionAuthManager) ClaimsFromRequest(r *http.Request) (*SessionAuthClaims, error) {
	cookie, err := r.Cookie(m.cookieName)
	if err != nil {
		slog.Debug("session auth cookie missing",
			"method", r.Method,
			"path", r.URL.Path,
		)
		return nil, ErrUnauthorized
	}
	if strings.TrimSpace(cookie.Value) == "" {
		slog.Debug("session auth cookie empty",
			"method", r.Method,
			"path", r.URL.Path,
		)
		return nil, ErrUnauthorized
	}
	claims, validateErr := m.ValidateToken(cookie.Value)
	if validateErr != nil {
		slog.Debug("session auth token validation failed",
			"method", r.Method,
			"path", r.URL.Path,
		)
		return nil, validateErr
	}
	return claims, nil
}

type sessionClaimsContextKey struct{}

// WithSessionAuthClaims writes claims to context for downstream handlers.
func WithSessionAuthClaims(ctx context.Context, claims *SessionAuthClaims) context.Context {
	return context.WithValue(ctx, sessionClaimsContextKey{}, claims)
}

// SessionAuthClaimsFromContext retrieves authenticated claims from request context.
func SessionAuthClaimsFromContext(ctx context.Context) (*SessionAuthClaims, bool) {
	claims, ok := ctx.Value(sessionClaimsContextKey{}).(*SessionAuthClaims)
	if !ok || claims == nil {
		return nil, false
	}
	return claims, true
}

// RequireSessionAuth validates the auth cookie and forwards claims in context.
func RequireSessionAuth(auth *SessionAuthManager, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if auth == nil {
			slog.Error("session auth middleware misconfigured",
				"method", r.Method,
				"path", r.URL.Path,
			)
			WriteError(w, ErrInternal, "Session auth is not configured", "AUTH_NOT_CONFIGURED")
			return
		}

		claims, err := auth.ClaimsFromRequest(r)
		if err != nil {
			slog.Debug("session auth rejected request",
				"method", r.Method,
				"path", r.URL.Path,
			)
			WriteError(w, ErrUnauthorized, "Unauthorized", "UNAUTHORIZED")
			return
		}
		slog.Debug("session auth accepted request",
			"method", r.Method,
			"path", r.URL.Path,
			"session_code", claims.SessionCode,
		)

		next(w, r.WithContext(WithSessionAuthClaims(r.Context(), claims)))
	}
}

// RequireSessionCodeMatch ensures the request session code matches the JWT claim.
func RequireSessionCodeMatch(w http.ResponseWriter, r *http.Request, sessionCode string) bool {
	claims, ok := SessionAuthClaimsFromContext(r.Context())
	if !ok {
		slog.Debug("session code match failed: missing auth claims",
			"method", r.Method,
			"path", r.URL.Path,
		)
		WriteError(w, ErrUnauthorized, "Unauthorized", "UNAUTHORIZED")
		return false
	}
	if strings.TrimSpace(sessionCode) == "" {
		slog.Debug("session code match failed: request missing session code",
			"method", r.Method,
			"path", r.URL.Path,
			"auth_session_code", claims.SessionCode,
		)
		WriteError(w, ErrBadRequest, "session_code is required", "BAD_REQUEST")
		return false
	}
	if strings.TrimSpace(sessionCode) != strings.TrimSpace(claims.SessionCode) {
		slog.Debug("session code mismatch",
			"method", r.Method,
			"path", r.URL.Path,
			"auth_session_code", claims.SessionCode,
			"request_session_code", sessionCode,
		)
		WriteError(w, ErrUnauthorized, "Unauthorized", "SESSION_MISMATCH")
		return false
	}
	return true
}
