package interview

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

func newTurnID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}
