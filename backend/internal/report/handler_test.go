package report

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/afirmativo/backend/internal/shared"
)

type fakeReportStore struct {
	getReportBySessionFn func(ctx context.Context, sessionCode string) (*Report, error)
	createReportFn       func(ctx context.Context, r *Report) error
	updateReportFn       func(ctx context.Context, r *Report) error
}

func (f *fakeReportStore) GetReportBySession(ctx context.Context, sessionCode string) (*Report, error) {
	if f.getReportBySessionFn != nil {
		return f.getReportBySessionFn(ctx, sessionCode)
	}
	return nil, nil
}

func (f *fakeReportStore) CreateReport(ctx context.Context, r *Report) error {
	if f.createReportFn != nil {
		return f.createReportFn(ctx, r)
	}
	return nil
}

func (f *fakeReportStore) UpdateReport(ctx context.Context, r *Report) error {
	if f.updateReportFn != nil {
		return f.updateReportFn(ctx, r)
	}
	return nil
}

type fakeReportSessionProvider struct {
	getSessionByCodeFn func(ctx context.Context, sessionCode string) (*SessionInfo, error)
}

func (f *fakeReportSessionProvider) GetSessionByCode(ctx context.Context, sessionCode string) (*SessionInfo, error) {
	if f.getSessionByCodeFn != nil {
		return f.getSessionByCodeFn(ctx, sessionCode)
	}
	return nil, nil
}

type fakeReportInterviewProvider struct{}

func (f *fakeReportInterviewProvider) GetAreasBySession(context.Context, string) ([]QuestionAreaRow, error) {
	return nil, nil
}

func (f *fakeReportInterviewProvider) GetAnswersBySession(context.Context, string) ([]AnswerRow, error) {
	return nil, nil
}

func (f *fakeReportInterviewProvider) GetAnswerCount(context.Context, string) (int, error) {
	return 0, nil
}

type fakeReportAIClient struct{}

func (f *fakeReportAIClient) GenerateReport(context.Context, []AreaSummary, string) (*ReportAIResponse, error) {
	return nil, fmt.Errorf("GenerateReport should not be called in these tests")
}

func withReportAuth(req *http.Request, sessionCode string) *http.Request {
	claims := &shared.SessionAuthClaims{SessionCode: sessionCode}
	return req.WithContext(shared.WithSessionAuthClaims(req.Context(), claims))
}

func decodeReportBody(t *testing.T, rr *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.Unmarshal(rr.Body.Bytes(), dst); err != nil {
		t.Fatalf("json.Unmarshal() error = %v; body=%s", err, rr.Body.String())
	}
}

func newReportHandlerForTest(store Store, sessions SessionProvider) *Handler {
	svc := NewService(store, &fakeReportInterviewProvider{}, sessions, &fakeReportAIClient{}, nil)
	return NewHandler(svc)
}

func TestServiceGetOrGenerateReport_MapsSessionErrorsToTypedSentinels(t *testing.T) {
	t.Parallel()

	store := &fakeReportStore{
		getReportBySessionFn: func(context.Context, string) (*Report, error) {
			return nil, nil
		},
	}
	sessions := &fakeReportSessionProvider{
		getSessionByCodeFn: func(context.Context, string) (*SessionInfo, error) {
			return nil, fmt.Errorf("db lookup failed: %w", shared.ErrNotFound)
		},
	}
	svc := NewService(store, &fakeReportInterviewProvider{}, sessions, &fakeReportAIClient{}, nil)

	_, err := svc.GetOrGenerateReport(context.Background(), "AP-AAAA-BBBB")
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("GetOrGenerateReport() error = %v, want ErrSessionNotFound", err)
	}
}

func TestServiceGetOrGenerateReport_RequiresCompletedSession(t *testing.T) {
	t.Parallel()

	store := &fakeReportStore{
		getReportBySessionFn: func(context.Context, string) (*Report, error) {
			return nil, nil
		},
	}
	sessions := &fakeReportSessionProvider{
		getSessionByCodeFn: func(context.Context, string) (*SessionInfo, error) {
			return &SessionInfo{SessionCode: "AP-AAAA-BBBB", Status: "interviewing"}, nil
		},
	}
	svc := NewService(store, &fakeReportInterviewProvider{}, sessions, &fakeReportAIClient{}, nil)

	_, err := svc.GetOrGenerateReport(context.Background(), "AP-AAAA-BBBB")
	if !errors.Is(err, ErrSessionNotCompleted) {
		t.Fatalf("GetOrGenerateReport() error = %v, want ErrSessionNotCompleted", err)
	}
}

func TestHandleGetReport_MissingCode(t *testing.T) {
	t.Parallel()

	h := newReportHandlerForTest(&fakeReportStore{}, &fakeReportSessionProvider{})
	req := httptest.NewRequest(http.MethodGet, "/api/report/", nil)
	rr := httptest.NewRecorder()

	h.HandleGetReport(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
	var got shared.ErrorResponse
	decodeReportBody(t, rr, &got)
	if got.Code != "MISSING_CODE" {
		t.Fatalf("code = %q, want MISSING_CODE", got.Code)
	}
}

func TestHandleGetReport_MapsTypedServiceErrors(t *testing.T) {
	t.Parallel()

	t.Run("session_not_found", func(t *testing.T) {
		t.Parallel()

		h := newReportHandlerForTest(
			&fakeReportStore{
				getReportBySessionFn: func(context.Context, string) (*Report, error) { return nil, nil },
			},
			&fakeReportSessionProvider{
				getSessionByCodeFn: func(context.Context, string) (*SessionInfo, error) {
					return nil, fmt.Errorf("wrapped: %w", shared.ErrNotFound)
				},
			},
		)

		req := httptest.NewRequest(http.MethodGet, "/api/report/AP-AAAA-BBBB", nil)
		req.SetPathValue("code", "AP-AAAA-BBBB")
		req = withReportAuth(req, "AP-AAAA-BBBB")
		rr := httptest.NewRecorder()
		h.HandleGetReport(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
		}
		var got shared.ErrorResponse
		decodeReportBody(t, rr, &got)
		if got.Code != "SESSION_NOT_FOUND" {
			t.Fatalf("code = %q, want SESSION_NOT_FOUND", got.Code)
		}
	})

	t.Run("session_not_completed", func(t *testing.T) {
		t.Parallel()

		h := newReportHandlerForTest(
			&fakeReportStore{
				getReportBySessionFn: func(context.Context, string) (*Report, error) { return nil, nil },
			},
			&fakeReportSessionProvider{
				getSessionByCodeFn: func(context.Context, string) (*SessionInfo, error) {
					return &SessionInfo{SessionCode: "AP-AAAA-BBBB", Status: "interviewing"}, nil
				},
			},
		)

		req := httptest.NewRequest(http.MethodGet, "/api/report/AP-AAAA-BBBB", nil)
		req.SetPathValue("code", "AP-AAAA-BBBB")
		req = withReportAuth(req, "AP-AAAA-BBBB")
		rr := httptest.NewRecorder()
		h.HandleGetReport(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
		}
		var got shared.ErrorResponse
		decodeReportBody(t, rr, &got)
		if got.Code != "NOT_COMPLETED" {
			t.Fatalf("code = %q, want NOT_COMPLETED", got.Code)
		}
	})
}

func TestHandleGetReport_TypedResponseContracts(t *testing.T) {
	t.Parallel()

	t.Run("generating", func(t *testing.T) {
		t.Parallel()

		h := newReportHandlerForTest(
			&fakeReportStore{
				getReportBySessionFn: func(context.Context, string) (*Report, error) {
					return &Report{SessionCode: "AP-AAAA-BBBB", Status: "generating"}, nil
				},
			},
			&fakeReportSessionProvider{},
		)

		req := httptest.NewRequest(http.MethodGet, "/api/report/AP-AAAA-BBBB", nil)
		req.SetPathValue("code", "AP-AAAA-BBBB")
		req = withReportAuth(req, "AP-AAAA-BBBB")
		rr := httptest.NewRecorder()
		h.HandleGetReport(rr, req)

		if rr.Code != http.StatusAccepted {
			t.Fatalf("status = %d, want %d", rr.Code, http.StatusAccepted)
		}
		var got struct {
			Status string `json:"status"`
		}
		decodeReportBody(t, rr, &got)
		if got.Status != "generating" {
			t.Fatalf("status = %q, want generating", got.Status)
		}
	})

	t.Run("ready", func(t *testing.T) {
		t.Parallel()

		h := newReportHandlerForTest(
			&fakeReportStore{
				getReportBySessionFn: func(context.Context, string) (*Report, error) {
					return &Report{
						SessionCode:     "AP-AAAA-BBBB",
						Status:          "ready",
						ContentEn:       "English report",
						ContentEs:       "Reporte en español",
						Strengths:       []string{"clarity"},
						Weaknesses:      []string{"pace"},
						Recommendation:  "continue practice",
						QuestionCount:   12,
						DurationMinutes: 31,
					}, nil
				},
			},
			&fakeReportSessionProvider{},
		)

		req := httptest.NewRequest(http.MethodGet, "/api/report/AP-AAAA-BBBB", nil)
		req.SetPathValue("code", "AP-AAAA-BBBB")
		req = withReportAuth(req, "AP-AAAA-BBBB")
		rr := httptest.NewRecorder()
		h.HandleGetReport(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
		}
		var got struct {
			SessionCode     string   `json:"session_code"`
			Status          string   `json:"status"`
			ContentEn       string   `json:"content_en"`
			ContentEs       string   `json:"content_es"`
			Strengths       []string `json:"strengths"`
			Weaknesses      []string `json:"weaknesses"`
			Recommendation  string   `json:"recommendation"`
			QuestionCount   int      `json:"question_count"`
			DurationMinutes int      `json:"duration_minutes"`
		}
		decodeReportBody(t, rr, &got)

		if got.SessionCode != "AP-AAAA-BBBB" {
			t.Fatalf("session_code = %q, want AP-AAAA-BBBB", got.SessionCode)
		}
		if got.Status != "ready" {
			t.Fatalf("status = %q, want ready", got.Status)
		}
		if got.ContentEn != "English report" || got.ContentEs != "Reporte en español" {
			t.Fatalf("content mismatch: en=%q es=%q", got.ContentEn, got.ContentEs)
		}
		if got.QuestionCount != 12 || got.DurationMinutes != 31 {
			t.Fatalf("question_count/duration_minutes = %d/%d, want 12/31", got.QuestionCount, got.DurationMinutes)
		}
	})

	t.Run("failed", func(t *testing.T) {
		t.Parallel()

		h := newReportHandlerForTest(
			&fakeReportStore{
				getReportBySessionFn: func(context.Context, string) (*Report, error) {
					return &Report{SessionCode: "AP-AAAA-BBBB", Status: "failed"}, nil
				},
			},
			&fakeReportSessionProvider{},
		)

		req := httptest.NewRequest(http.MethodGet, "/api/report/AP-AAAA-BBBB", nil)
		req.SetPathValue("code", "AP-AAAA-BBBB")
		req = withReportAuth(req, "AP-AAAA-BBBB")
		rr := httptest.NewRecorder()
		h.HandleGetReport(rr, req)

		if rr.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
		}
		var got shared.ErrorResponse
		decodeReportBody(t, rr, &got)
		if got.Code != "GENERATION_FAILED" {
			t.Fatalf("code = %q, want GENERATION_FAILED", got.Code)
		}
	})
}
