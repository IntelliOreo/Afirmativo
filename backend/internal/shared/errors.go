// Sentinel errors for consistent error-to-HTTP-status mapping across handlers.
// Use errors.Is to check these in handler code.
package shared

import "errors"

var (
	ErrNotFound      = errors.New("not found")
	ErrUnauthorized  = errors.New("unauthorized")
	ErrGone          = errors.New("gone")
	ErrConflict      = errors.New("conflict")
	ErrBadRequest    = errors.New("bad request")
	ErrRateLimited   = errors.New("rate limited")
	ErrInternal      = errors.New("internal error")
	ErrCouponInvalid = errors.New("coupon invalid or already used")
)
