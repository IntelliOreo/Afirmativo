package shared

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
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
	var buf bytes.Buffer
	setDefaultLogger(t, slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))

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

func TestRecovery_ReturnsInternalServerErrorBeforeResponseStarts(t *testing.T) {
	t.Parallel()

	handler := Recovery(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content-type = %q, want application/json", got)
	}

	var got ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if got.Error != "Internal server error" {
		t.Fatalf("error = %q, want Internal server error", got.Error)
	}
	if got.Code != "INTERNAL_ERROR" {
		t.Fatalf("code = %q, want INTERNAL_ERROR", got.Code)
	}
}

func TestRequestIDLoggerRecovery_PreservesRequestIDAndLogsStatus500(t *testing.T) {
	var buf bytes.Buffer
	setDefaultLogger(t, slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))

	handler := RequestID(Logger(Recovery(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	}))))

	req := httptest.NewRequest(http.MethodPost, "/api/test", nil)
	req.RemoteAddr = "203.0.113.10:1234"
	req.Header.Set(RequestIDHeader, "req-logger")
	req.Header.Set("User-Agent", "test-agent")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
	if got := rr.Header().Get(RequestIDHeader); got != "req-logger" {
		t.Fatalf("response request id = %q, want req-logger", got)
	}

	records := decodeJSONLogRecords(t, &buf)
	completed := findJSONLogRecord(t, records, "request completed")
	if got := completed["request_id"]; got != "req-logger" {
		t.Fatalf("request completed request_id = %v, want req-logger", got)
	}
	if got := completed["status"]; got != float64(http.StatusInternalServerError) {
		t.Fatalf("request completed status = %v, want %d", got, http.StatusInternalServerError)
	}
}

func TestLoggerRecovery_DoesNotRewriteCommittedResponse(t *testing.T) {
	var buf bytes.Buffer
	setDefaultLogger(t, slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))

	handler := Logger(Recovery(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		if _, err := w.Write([]byte("partial")); err != nil {
			t.Fatalf("write partial response: %v", err)
		}
		panic("boom")
	})))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusAccepted)
	}
	if got := rr.Body.String(); got != "partial" {
		t.Fatalf("body = %q, want partial", got)
	}
	if strings.Contains(rr.Body.String(), "INTERNAL_ERROR") {
		t.Fatalf("body unexpectedly contains recovery error payload: %q", rr.Body.String())
	}

	records := decodeJSONLogRecords(t, &buf)
	recovered := findJSONLogRecord(t, records, "recovered panic")
	if got := recovered["panic"]; got != "boom" {
		t.Fatalf("recovered panic value = %v, want boom", got)
	}
}

func TestRecovery_LogsStackTraceWithinCap(t *testing.T) {
	var buf bytes.Buffer
	setDefaultLogger(t, slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))

	handler := Recovery(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panicWithDeepStack(2048)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	records := decodeJSONLogRecords(t, &buf)
	recovered := findJSONLogRecord(t, records, "recovered panic")
	stack, ok := recovered["stack"].(string)
	if !ok {
		t.Fatalf("stack field type = %T, want string", recovered["stack"])
	}
	if len(stack) == 0 {
		t.Fatalf("stack field was empty")
	}
	if len(stack) > maxRecoveryStackBytes {
		t.Fatalf("stack length = %d, want <= %d", len(stack), maxRecoveryStackBytes)
	}
	if len(stack) != maxRecoveryStackBytes {
		t.Fatalf("stack length = %d, want exact cap %d for deep stack", len(stack), maxRecoveryStackBytes)
	}
}

func TestTraceLoggerRecovery_RecordsRecoveredPanicAs500(t *testing.T) {
	setDefaultLogger(t, slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError})))

	recorder := tracetest.NewSpanRecorder()
	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previousProvider := otel.GetTracerProvider()
	otel.SetTracerProvider(tracerProvider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		_ = tracerProvider.Shutdown(context.Background())
	})

	handler := Trace(Logger(Recovery(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	}))))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("ended spans = %d, want 1", len(spans))
	}

	var statusCode int64 = -1
	for _, attr := range spans[0].Attributes() {
		if string(attr.Key) == "http.status_code" {
			statusCode = attr.Value.AsInt64()
			break
		}
	}
	if statusCode != http.StatusInternalServerError {
		t.Fatalf("http.status_code = %d, want %d", statusCode, http.StatusInternalServerError)
	}
}

func setDefaultLogger(t *testing.T, logger *slog.Logger) {
	t.Helper()

	previous := slog.Default()
	slog.SetDefault(logger)
	t.Cleanup(func() {
		slog.SetDefault(previous)
	})
}

func decodeJSONLogRecords(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()

	dec := json.NewDecoder(strings.NewReader(buf.String()))
	var records []map[string]any
	for {
		var record map[string]any
		err := dec.Decode(&record)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("decode log record: %v", err)
		}
		records = append(records, record)
	}
	if len(records) == 0 {
		t.Fatalf("no log records captured")
	}
	return records
}

func findJSONLogRecord(t *testing.T, records []map[string]any, msg string) map[string]any {
	t.Helper()

	for _, record := range records {
		if record["msg"] == msg {
			return record
		}
	}
	t.Fatalf("log record %q not found in %#v", msg, records)
	return nil
}

func panicWithDeepStack(depth int) {
	if depth == 0 {
		panic("deep boom")
	}
	panicWithDeepStack(depth - 1)
}
