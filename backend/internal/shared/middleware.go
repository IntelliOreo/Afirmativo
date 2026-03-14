// HTTP middleware: CORS, RequestID, Logger, SecurityHeaders.
// Applied in cmd/server/main.go via middleware chain.
package shared

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// CORS returns middleware that sets CORS headers for the given origin.
func CORS(allowedOrigin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Request-Id")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Expose-Headers", RequestIDHeader)
			w.Header().Set("Access-Control-Max-Age", "86400")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// SecurityHeaders adds standard security headers to all responses.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		next.ServeHTTP(w, r)
	})
}

// Logger logs each request with method, path, status, and duration.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIP := ClientIPFromRequest(r)
		requestID := RequestIDFromContext(r.Context())
		slog.Debug("request received",
			"request_id", requestID,
			"method", r.Method,
			"path", r.URL.Path,
			"client_ip", clientIP,
			"user_agent", r.UserAgent(),
			"content_length", r.ContentLength,
		)
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		slog.Info("request completed",
			"request_id", requestID,
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"client_ip", clientIP,
			"user_agent", r.UserAgent(),
		)
	})
}

// Trace creates a span per HTTP request using the global OTel tracer.
// When OTel is disabled (noop provider), this adds negligible overhead.
func Trace(next http.Handler) http.Handler {
	tracer := otel.Tracer("afirmativo-http")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		spanName := fmt.Sprintf("%s %s", r.Method, r.URL.Path)
		ctx, span := tracer.Start(r.Context(), spanName,
			trace.WithSpanKind(trace.SpanKindServer),
		)
		defer span.End()

		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r.WithContext(ctx))

		span.SetAttributes(
			attribute.Int("http.status_code", sw.status),
			attribute.String("http.method", r.Method),
			attribute.String("http.target", r.URL.Path),
		)
	})
}

// Chain applies middleware in order (outermost first).
func Chain(h http.Handler, mw ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mw) - 1; i >= 0; i-- {
		h = mw[i](h)
	}
	return h
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}
