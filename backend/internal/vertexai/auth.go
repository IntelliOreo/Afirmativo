package vertexai

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const googleOAuthScope = "https://www.googleapis.com/auth/cloud-platform"

type tokenSource interface {
	Token(ctx context.Context) (string, error)
}

type adcTokenSource struct {
	httpClient *http.Client
	nowFn      func() time.Time

	mu          sync.Mutex
	cachedToken string
	expiry      time.Time

	serviceAccount *serviceAccountCredentials
	authorizedUser *authorizedUserCredentials
}

type serviceAccountCredentials struct {
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

type authorizedUserCredentials struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RefreshToken string `json:"refresh_token"`
	TokenURI     string `json:"token_uri"`
}

func newADCTokenSource(httpClient *http.Client, nowFn func() time.Time) (tokenSource, error) {
	raw, err := loadADCCredentials()
	if err != nil {
		return nil, err
	}

	var envelope struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("parse ADC credentials: %w", err)
	}

	source := &adcTokenSource{
		httpClient: httpClient,
		nowFn:      nowFn,
	}
	switch strings.TrimSpace(envelope.Type) {
	case "service_account":
		var creds serviceAccountCredentials
		if err := json.Unmarshal(raw, &creds); err != nil {
			return nil, fmt.Errorf("parse service account ADC credentials: %w", err)
		}
		if creds.TokenURI == "" {
			creds.TokenURI = "https://oauth2.googleapis.com/token"
		}
		source.serviceAccount = &creds
	case "authorized_user":
		var creds authorizedUserCredentials
		if err := json.Unmarshal(raw, &creds); err != nil {
			return nil, fmt.Errorf("parse authorized user ADC credentials: %w", err)
		}
		if creds.TokenURI == "" {
			creds.TokenURI = "https://oauth2.googleapis.com/token"
		}
		source.authorizedUser = &creds
	default:
		return nil, fmt.Errorf("unsupported ADC credential type %q", envelope.Type)
	}
	return source, nil
}

func (s *adcTokenSource) Token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cachedToken != "" && s.expiry.After(s.nowFn().Add(30*time.Second)) {
		return s.cachedToken, nil
	}

	var (
		token  string
		expiry time.Time
		err    error
	)
	switch {
	case s.serviceAccount != nil:
		token, expiry, err = s.exchangeServiceAccountToken(ctx)
	case s.authorizedUser != nil:
		token, expiry, err = s.refreshAuthorizedUserToken(ctx)
	default:
		err = fmt.Errorf("no ADC credentials loaded")
	}
	if err != nil {
		return "", err
	}

	s.cachedToken = token
	s.expiry = expiry
	return token, nil
}

func (s *adcTokenSource) exchangeServiceAccountToken(ctx context.Context) (string, time.Time, error) {
	creds := s.serviceAccount
	assertion, err := buildServiceAccountJWT(creds, s.nowFn())
	if err != nil {
		return "", time.Time{}, err
	}

	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer")
	form.Set("assertion", assertion)
	return s.exchangeOAuthToken(ctx, creds.TokenURI, form)
}

func (s *adcTokenSource) refreshAuthorizedUserToken(ctx context.Context) (string, time.Time, error) {
	creds := s.authorizedUser
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", creds.ClientID)
	form.Set("client_secret", creds.ClientSecret)
	form.Set("refresh_token", creds.RefreshToken)
	return s.exchangeOAuthToken(ctx, creds.TokenURI, form)
}

func (s *adcTokenSource) exchangeOAuthToken(ctx context.Context, tokenURL string, form url.Values) (string, time.Time, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("build OAuth token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("OAuth token exchange failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("read OAuth token response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", time.Time{}, fmt.Errorf("OAuth token exchange status %d", resp.StatusCode)
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(raw, &tokenResp); err != nil {
		return "", time.Time{}, fmt.Errorf("decode OAuth token response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return "", time.Time{}, fmt.Errorf("OAuth token response missing access_token")
	}
	expiry := s.nowFn().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	return tokenResp.AccessToken, expiry, nil
}

func loadADCCredentials() ([]byte, error) {
	if explicitPath := strings.TrimSpace(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")); explicitPath != "" {
		raw, err := os.ReadFile(explicitPath)
		if err != nil {
			return nil, fmt.Errorf("read GOOGLE_APPLICATION_CREDENTIALS: %w", err)
		}
		return raw, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home directory for ADC lookup: %w", err)
	}
	defaultPath := filepath.Join(homeDir, ".config", "gcloud", "application_default_credentials.json")
	raw, err := os.ReadFile(defaultPath)
	if err != nil {
		return nil, fmt.Errorf("read ADC credentials: %w", err)
	}
	return raw, nil
}

func buildServiceAccountJWT(creds *serviceAccountCredentials, now time.Time) (string, error) {
	privateKey, err := parseRSAPrivateKey(creds.PrivateKey)
	if err != nil {
		return "", err
	}

	headerJSON, err := json.Marshal(map[string]string{
		"alg": "RS256",
		"typ": "JWT",
	})
	if err != nil {
		return "", fmt.Errorf("marshal JWT header: %w", err)
	}
	claimsJSON, err := json.Marshal(map[string]any{
		"iss":   creds.ClientEmail,
		"scope": googleOAuthScope,
		"aud":   creds.TokenURI,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	})
	if err != nil {
		return "", fmt.Errorf("marshal JWT claims: %w", err)
	}

	encodedHeader := base64.RawURLEncoding.EncodeToString(headerJSON)
	encodedClaims := base64.RawURLEncoding.EncodeToString(claimsJSON)
	signingInput := encodedHeader + "." + encodedClaims

	hash := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, hash[:])
	if err != nil {
		return "", fmt.Errorf("sign JWT assertion: %w", err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func parseRSAPrivateKey(raw string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(raw))
	if block == nil {
		return nil, fmt.Errorf("decode PEM private key")
	}

	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("private key is not RSA")
		}
		return rsaKey, nil
	}

	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse RSA private key: %w", err)
	}
	return key, nil
}
