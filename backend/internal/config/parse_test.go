package config

import (
	"strings"
	"testing"
)

func TestEnvInt_Invalid(t *testing.T) {
	t.Setenv("TEST_INT", "abc")

	_, err := envInt("TEST_INT", 5)
	if err == nil {
		t.Fatal("envInt() expected error")
	}
	if !strings.Contains(err.Error(), "invalid TEST_INT:") {
		t.Fatalf("envInt() error = %v", err)
	}
}

func TestEnvIntMin_PreservesPositiveValidationMessage(t *testing.T) {
	t.Setenv("TEST_MIN_INT", "0")

	_, err := envIntMin("TEST_MIN_INT", 5, 1)
	if err == nil {
		t.Fatal("envIntMin() expected error")
	}
	if err.Error() != "TEST_MIN_INT must be > 0" {
		t.Fatalf("envIntMin() error = %q", err.Error())
	}
}

func TestEnvFloat_RangeValidationMessage(t *testing.T) {
	t.Setenv("TEST_FLOAT", "3")

	_, err := envFloat("TEST_FLOAT", 0.3, 0, 2)
	if err == nil {
		t.Fatal("envFloat() expected error")
	}
	if err.Error() != "TEST_FLOAT must be between 0 and 2" {
		t.Fatalf("envFloat() error = %q", err.Error())
	}
}

func TestEnvBool_Invalid(t *testing.T) {
	t.Setenv("TEST_BOOL", "nope")

	_, err := envBool("TEST_BOOL", false)
	if err == nil {
		t.Fatal("envBool() expected error")
	}
	if !strings.Contains(err.Error(), "invalid TEST_BOOL:") {
		t.Fatalf("envBool() error = %v", err)
	}
}
