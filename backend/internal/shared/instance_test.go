package shared

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResolveInstanceMetadata(t *testing.T) {
	t.Parallel()

	t.Run("instance_id_override", func(t *testing.T) {
		t.Parallel()

		meta := resolveInstanceMetadata(func(key string) string {
			switch key {
			case "INSTANCE_ID":
				return "instance-override"
			case "K_SERVICE":
				return "svc"
			case "K_REVISION":
				return "rev"
			case "HOSTNAME":
				return "host"
			default:
				return ""
			}
		}, func() (string, error) {
			return "ignored", nil
		})

		if meta.ID != "instance-override" {
			t.Fatalf("ID = %q, want instance-override", meta.ID)
		}
	})

	t.Run("cloud_run_shape", func(t *testing.T) {
		t.Parallel()

		meta := resolveInstanceMetadata(func(key string) string {
			switch key {
			case "K_SERVICE":
				return "afirmativo-backend"
			case "K_REVISION":
				return "afirmativo-backend-00012-abc"
			case "HOSTNAME":
				return "backend-host"
			default:
				return ""
			}
		}, nil)

		if meta.ID != "afirmativo-backend/afirmativo-backend-00012-abc/backend-host" {
			t.Fatalf("ID = %q, want combined cloud-run identifier", meta.ID)
		}
	})

	t.Run("hostname_fallback", func(t *testing.T) {
		t.Parallel()

		meta := resolveInstanceMetadata(func(string) string { return "" }, func() (string, error) {
			return "local-host", nil
		})

		if meta.ID != "local-host" {
			t.Fatalf("ID = %q, want local-host", meta.ID)
		}
	})

	t.Run("unknown_when_unavailable", func(t *testing.T) {
		t.Parallel()

		meta := resolveInstanceMetadata(func(string) string { return "" }, func() (string, error) {
			return "", errors.New("no hostname")
		})

		if meta.ID != "unknown" {
			t.Fatalf("ID = %q, want unknown", meta.ID)
		}
	})
}

type stubHealthDB struct {
	err error
}

func (s stubHealthDB) Ping(context.Context) error {
	return s.err
}

type stubHealthProvider struct {
	stats map[string]any
}

func (s stubHealthProvider) HealthStats() map[string]any {
	return s.stats
}

func TestHandleHealth_InstanceMetadata(t *testing.T) {
	t.Parallel()

	handler := HandleHealth(
		stubHealthDB{},
		"test-version",
		InstanceMetadata{ID: "instance-1", Revision: "rev-1"},
		stubHealthProvider{stats: map[string]any{"async_answer_queue_depth": 2}},
	)

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rr := httptest.NewRecorder()
	handler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	body := rr.Body.String()
	for _, want := range []string{
		`"instance_id":"instance-1"`,
		`"health_scope":"instance_local"`,
		`"service_revision":"rev-1"`,
		`"async_answer_queue_depth":2`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body = %s, want substring %s", body, want)
		}
	}
}
