// Package session handles coupon validation and session management.
// This file defines the Session domain type and helpers for generating
// session codes (AP-XXXX-XXXX) and 6-digit PINs.
// No infrastructure imports — domain types are infrastructure-free.
package session

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"time"
)

var (
	ErrSessionExpired = errors.New("session expired")
	ErrPINIncorrect   = errors.New("PIN incorrect")
)

// Session represents an active user session.
type Session struct {
	SessionCode  string
	PinHash      string
	Track        string
	Status       string
	Role         string
	TimerSeconds int
	StartedAt    *time.Time
	EndedAt      *time.Time
	PaymentID    string
	CouponCode   string
	ExpiresAt    time.Time
	CreatedAt    time.Time
}

// Coupon represents a redeemable coupon.
type Coupon struct {
	Code        string
	MaxUses     int
	CurrentUses int
	DiscountPct int
	ExpiresAt   *time.Time
}

const codeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no I/O/0/1 to avoid confusion

// GenerateSessionCode produces a session code in the format AP-XXXX-XXXX.
func GenerateSessionCode() (string, error) {
	seg1, err := randomString(4, codeAlphabet)
	if err != nil {
		return "", fmt.Errorf("generate session code segment 1: %w", err)
	}
	seg2, err := randomString(4, codeAlphabet)
	if err != nil {
		return "", fmt.Errorf("generate session code segment 2: %w", err)
	}
	return fmt.Sprintf("AP-%s-%s", seg1, seg2), nil
}

// GeneratePIN produces a 6-digit numeric PIN.
func GeneratePIN() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", fmt.Errorf("generate PIN: %w", err)
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

func randomString(length int, alphabet string) (string, error) {
	b := make([]byte, length)
	max := big.NewInt(int64(len(alphabet)))
	for i := range b {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b[i] = alphabet[n.Int64()]
	}
	return string(b), nil
}
