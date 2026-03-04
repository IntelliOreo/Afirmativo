package session

import (
	"regexp"
	"testing"
)

func TestGenerateSessionCodeFormat(t *testing.T) {
	t.Parallel()

	re := regexp.MustCompile(`^AP-[A-HJ-NP-Z2-9]{4}-[A-HJ-NP-Z2-9]{4}$`)

	for i := 0; i < 100; i++ {
		code, err := GenerateSessionCode()
		if err != nil {
			t.Fatalf("GenerateSessionCode() error = %v", err)
		}
		if !re.MatchString(code) {
			t.Fatalf("GenerateSessionCode() = %q, want AP-XXXX-XXXX using safe alphabet", code)
		}
	}
}

func TestGeneratePINFormat(t *testing.T) {
	t.Parallel()

	re := regexp.MustCompile(`^[0-9]{6}$`)

	for i := 0; i < 100; i++ {
		pin, err := GeneratePIN()
		if err != nil {
			t.Fatalf("GeneratePIN() error = %v", err)
		}
		if !re.MatchString(pin) {
			t.Fatalf("GeneratePIN() = %q, want 6 digits", pin)
		}
	}
}
