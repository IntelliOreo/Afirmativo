package shared

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestID_GeneratesAndEchoesHeader(t *testing.T) {
	t.Parallel()

	var seenRequestID string
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenRequestID = RequestIDFromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNoContent)
	}
	if rr.Header().Get(RequestIDHeader) == "" {
		t.Fatalf("response %s header was empty", RequestIDHeader)
	}
	if seenRequestID == "" {
		t.Fatalf("request id was not stored in context")
	}
	if seenRequestID != rr.Header().Get(RequestIDHeader) {
		t.Fatalf("context request id = %q, want %q", seenRequestID, rr.Header().Get(RequestIDHeader))
	}
}

func TestRequestID_PropagatesInboundHeader(t *testing.T) {
	t.Parallel()

	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := RequestIDFromContext(r.Context()); got != "req-abc" {
			t.Fatalf("context request id = %q, want req-abc", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set(RequestIDHeader, "req-abc")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if got := rr.Header().Get(RequestIDHeader); got != "req-abc" {
		t.Fatalf("response request id = %q, want req-abc", got)
	}
}

func TestLogger_IncludesRequestIDInStructuredLog(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	previous := slog.Default()
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	slog.SetDefault(logger)
	t.Cleanup(func() {
		slog.SetDefault(previous)
	})

	handler := Logger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/test", nil)
	req.RemoteAddr = "203.0.113.10:1234"
	req.Header.Set("User-Agent", "test-agent")
	req = req.WithContext(WithRequestID(context.Background(), "req-logger"))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	logOutput := buf.String()
	for _, fragment := range []string{
		"request_id=req-logger",
		"method=POST",
		"path=/api/test",
		"status=201",
		"client_ip=203.0.113.10",
		"user_agent=test-agent",
	} {
		if !strings.Contains(logOutput, fragment) {
			t.Fatalf("log output missing %q: %s", fragment, logOutput)
		}
	}
}
