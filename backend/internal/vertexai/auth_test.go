package vertexai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewADCTokenSource_UsesMetadataFallbackWhenNoFileCredentialsExist(t *testing.T) {
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
	t.Setenv("HOME", t.TempDir())

	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != defaultMetadataTokenURL {
			t.Fatalf("request URL = %q, want %q", r.URL.String(), defaultMetadataTokenURL)
		}
		if got := r.Header.Get("Metadata-Flavor"); got != "Google" {
			t.Fatalf("Metadata-Flavor = %q, want Google", got)
		}
		return jsonHTTPResponse(http.StatusOK, map[string]any{
			"access_token": "metadata-token",
			"expires_in":   1200,
		}), nil
	})}

	source, err := newADCTokenSource(httpClient, time.Now)
	if err != nil {
		t.Fatalf("newADCTokenSource() error = %v", err)
	}

	token, err := source.Token(context.Background())
	if err != nil {
		t.Fatalf("Token() error = %v", err)
	}
	if token != "metadata-token" {
		t.Fatalf("token = %q, want metadata-token", token)
	}
}

func TestNewADCTokenSource_UsesExplicitCredentialsFileBeforeMetadata(t *testing.T) {
	credsDir := t.TempDir()
	credsPath := filepath.Join(credsDir, "adc.json")
	if err := os.WriteFile(credsPath, []byte(`{
  "type": "authorized_user",
  "client_id": "client-id",
  "client_secret": "client-secret",
  "refresh_token": "refresh-token",
  "token_uri": "https://oauth2.test/token"
}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", credsPath)
	t.Setenv("HOME", t.TempDir())

	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://oauth2.test/token" {
			t.Fatalf("request URL = %q, want https://oauth2.test/token", r.URL.String())
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		if !strings.Contains(string(body), "refresh_token=refresh-token") {
			t.Fatalf("request body = %q, want refresh token", string(body))
		}
		return jsonHTTPResponse(http.StatusOK, map[string]any{
			"access_token": "file-token",
			"expires_in":   1800,
		}), nil
	})}

	source, err := newADCTokenSource(httpClient, time.Now)
	if err != nil {
		t.Fatalf("newADCTokenSource() error = %v", err)
	}

	token, err := source.Token(context.Background())
	if err != nil {
		t.Fatalf("Token() error = %v", err)
	}
	if token != "file-token" {
		t.Fatalf("token = %q, want file-token", token)
	}
}

func jsonHTTPResponse(statusCode int, body any) *http.Response {
	raw, _ := json.Marshal(body)
	return &http.Response{
		StatusCode: statusCode,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(string(raw))),
	}
}
