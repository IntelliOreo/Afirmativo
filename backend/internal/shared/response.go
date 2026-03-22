// Package shared provides cross-cutting infrastructure used by all domain packages.
// This file contains JSON response helpers: WriteJSON and WriteError.
package shared

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
)

// ErrorResponse is the standard error shape returned by all endpoints.
type ErrorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
}

// WriteJSON writes a JSON response with the given status code.
func WriteJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("failed to encode JSON response", "error", err)
	}
}

// WriteError maps a sentinel error to an HTTP status and writes a JSON error response.
func WriteError(w http.ResponseWriter, err error, msg, code string) {
	status := mapErrorStatus(err)
	WriteErrorStatus(w, status, msg, code)
}

// WriteErrorStatus writes a JSON error response for handlers with fixed statuses.
func WriteErrorStatus(w http.ResponseWriter, status int, msg, code string) {
	WriteJSON(w, status, ErrorResponse{
		Error: msg,
		Code:  code,
	})
}

// mapErrorStatus returns the HTTP status code for a given sentinel error.
func mapErrorStatus(err error) int {
	switch {
	case errors.Is(err, ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, ErrUnauthorized):
		return http.StatusUnauthorized
	case errors.Is(err, ErrGone):
		return http.StatusGone
	case errors.Is(err, ErrConflict):
		return http.StatusConflict
	case errors.Is(err, ErrBadRequest), errors.Is(err, ErrCouponInvalid):
		return http.StatusBadRequest
	case errors.Is(err, ErrRateLimited):
		return http.StatusTooManyRequests
	default:
		return http.StatusInternalServerError
	}
}

// DecodeJSON reads a JSON request body into dst, enforcing a size limit.
func DecodeJSON(r *http.Request, dst any, maxBytes int64) error {
	r.Body = http.MaxBytesReader(nil, r.Body, maxBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}
