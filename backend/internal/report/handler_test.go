package report

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/afirmativo/backend/internal/config"
	"github.com/afirmativo/backend/internal/shared"
)

type fakeReportStore struct {
	getReportBySessionFn           func(ctx context.Context, sessionCode string) (*Report, error)
	createReportFn                 func(ctx context.Context, r *Report) error
	setReportQueuedFn              func(ctx context.Context, sessionCode string, resetAttempts bool) error
	claimQueuedReportFn            func(ctx context.Context, sessionCode string) (*Report, error)
	listQueuedReportSessionCodesFn func(ctx context.Context, limit int) ([]string, error)
	requeueStaleRunningReportsFn   func(ctx context.Context, staleBefore time.Time) (int64, error)
	markReportReadyFn              func(ctx context.Context, r *Report) error
	markReportFailedFn             func(ctx context.Context, sessionCode, errorCode, errorMessage string) error
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

func (f *fakeReportStore) SetReportQueued(ctx context.Context, sessionCode string, resetAttempts bool) error {
	if f.setReportQueuedFn != nil {
		return f.setReportQueuedFn(ctx, sessionCode, resetAttempts)
	}
	return nil
}

func (f *fakeReportStore) ClaimQueuedReport(ctx context.Context, sessionCode string) (*Report, error) {
	if f.claimQueuedReportFn != nil {
		return f.claimQueuedReportFn(ctx, sessionCode)
	}
	return nil, nil
}

func (f *fakeReportStore) ListQueuedReportSessionCodes(ctx context.Context, limit int) ([]string, error) {
	if f.listQueuedReportSessionCodesFn != nil {
		return f.listQueuedReportSessionCodesFn(ctx, limit)
	}
	return nil, nil
}

func (f *fakeReportStore) RequeueStaleRunningReports(ctx context.Context, staleBefore time.Time) (int64, error) {
	if f.requeueStaleRunningReportsFn != nil {
		return f.requeueStaleRunningReportsFn(ctx, staleBefore)
	}
	return 0, nil
}

func (f *fakeReportStore) MarkReportReady(ctx context.Context, r *Report) error {
	if f.markReportReadyFn != nil {
		return f.markReportReadyFn(ctx, r)
	}
	return nil
}

func (f *fakeReportStore) MarkReportFailed(ctx context.Context, sessionCode, errorCode, errorMessage string) error {
	if f.markReportFailedFn != nil {
		return f.markReportFailedFn(ctx, sessionCode, errorCode, errorMessage)
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

func (f *fakeReportInterviewProvider) GetAreasBySession(context.Context, string) ([]InterviewAreaSnapshot, error) {
	return nil, nil
}

func (f *fakeReportInterviewProvider) GetAnswersBySession(context.Context, string) ([]InterviewAnswerSnapshot, error) {
	return nil, nil
}

func (f *fakeReportInterviewProvider) GetAnswerCount(context.Context, string) (int, error) {
	return 0, nil
}

type fakeReportAIClient struct {
	generateReportFn func(context.Context, []AreaSummary, string) (*ReportAIResponse, error)
}

func (f *fakeReportAIClient) GenerateReport(ctx context.Context, summaries []AreaSummary, transcript string) (*ReportAIResponse, error) {
	if f.generateReportFn != nil {
		return f.generateReportFn(ctx, summaries, transcript)
	}
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

func defaultReportSettings(areaConfigs []config.AreaConfig) Settings {
	return Settings{
		AreaConfigs: areaConfigs,
		DBTimeout:   5 * time.Second,
		AsyncRuntime: config.AsyncRuntimeConfig{
			Workers:       2,
			QueueSize:     64,
			RecoveryBatch: 50,
			RecoveryEvery: 10 * time.Second,
			StaleAfter:    3 * time.Minute,
			JobTimeout:    3 * time.Minute,
		},
	}
}

func newReportHandlerForTest(store Store, sessions SessionProvider) *Handler {
	svc := NewService(Deps{
		Store:      store,
		Interviews: &fakeReportInterviewProvider{},
		Sessions:   sessions,
		AIClient:   &fakeReportAIClient{},
	}, defaultReportSettings(nil))
	return NewHandler(svc)
}

func TestServiceGetReport_MapsSessionErrorsToTypedSentinels(t *testing.T) {
	t.Parallel()

	store := &fakeReportStore{}
	sessions := &fakeReportSessionProvider{
		getSessionByCodeFn: func(context.Context, string) (*SessionInfo, error) {
			return nil, fmt.Errorf("db lookup failed: %w", shared.ErrNotFound)
		},
	}
	svc := NewService(Deps{
		Store:      store,
		Interviews: &fakeReportInterviewProvider{},
		Sessions:   sessions,
		AIClient:   &fakeReportAIClient{},
	}, defaultReportSettings(nil))

	_, err := svc.GetReport(context.Background(), "AP-AAAA-BBBB")
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("GetReport() error = %v, want ErrSessionNotFound", err)
	}
}

func TestServiceGenerateReportAsync_QueuesFailedReport(t *testing.T) {
	t.Parallel()

	queued := false
	store := &fakeReportStore{
		getReportBySessionFn: func(context.Context, string) (*Report, error) {
			return &Report{SessionCode: "AP-AAAA-BBBB", Status: ReportStatusFailed}, nil
		},
		setReportQueuedFn: func(context.Context, string, bool) error {
			queued = true
			return nil
		},
	}
	sessions := &fakeReportSessionProvider{
		getSessionByCodeFn: func(context.Context, string) (*SessionInfo, error) {
			return &SessionInfo{SessionCode: "AP-AAAA-BBBB", Status: "completed"}, nil
		},
	}
	svc := NewService(Deps{
		Store:      store,
		Interviews: &fakeReportInterviewProvider{},
		Sessions:   sessions,
		AIClient:   &fakeReportAIClient{},
	}, defaultReportSettings(nil))

	report, err := svc.GenerateReportAsync(context.Background(), "AP-AAAA-BBBB")
	if err != nil {
		t.Fatalf("GenerateReportAsync() error = %v", err)
	}
	if !queued {
		t.Fatalf("expected failed report to be queued for retry")
	}
	if report == nil || report.Status != ReportStatusQueued {
		t.Fatalf("report = %#v, want queued", report)
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
}

func TestHandleGetReport_MapsTypedServiceErrors(t *testing.T) {
	t.Parallel()

	t.Run("session_not_found", func(t *testing.T) {
		h := newReportHandlerForTest(
			&fakeReportStore{},
			&fakeReportSessionProvider{
				getSessionByCodeFn: func(context.Context, string) (*SessionInfo, error) {
					return nil, fmt.Errorf("wrapped: %w", shared.ErrNotFound)
				},
			},
		)

		req := withReportAuth(httptest.NewRequest(http.MethodGet, "/api/report/AP-AAAA-BBBB", nil), "AP-AAAA-BBBB")
		req.SetPathValue("code", "AP-AAAA-BBBB")
		rr := httptest.NewRecorder()
		h.HandleGetReport(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
		}
	})

	t.Run("report_not_started", func(t *testing.T) {
		h := newReportHandlerForTest(
			&fakeReportStore{},
			&fakeReportSessionProvider{
				getSessionByCodeFn: func(context.Context, string) (*SessionInfo, error) {
					return &SessionInfo{SessionCode: "AP-AAAA-BBBB", Status: "completed"}, nil
				},
			},
		)

		req := withReportAuth(httptest.NewRequest(http.MethodGet, "/api/report/AP-AAAA-BBBB", nil), "AP-AAAA-BBBB")
		req.SetPathValue("code", "AP-AAAA-BBBB")
		rr := httptest.NewRecorder()
		h.HandleGetReport(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
		}
		var got shared.ErrorResponse
		decodeReportBody(t, rr, &got)
		if got.Code != "REPORT_NOT_STARTED" {
			t.Fatalf("code = %q, want REPORT_NOT_STARTED", got.Code)
		}
	})
}

func TestHandleGetReport_TypedResponseContracts(t *testing.T) {
	t.Parallel()

	t.Run("generating", func(t *testing.T) {
		h := newReportHandlerForTest(
			&fakeReportStore{
				getReportBySessionFn: func(context.Context, string) (*Report, error) {
					return &Report{SessionCode: "AP-AAAA-BBBB", Status: ReportStatusQueued}, nil
				},
			},
			&fakeReportSessionProvider{},
		)

		req := withReportAuth(httptest.NewRequest(http.MethodGet, "/api/report/AP-AAAA-BBBB", nil), "AP-AAAA-BBBB")
		req.SetPathValue("code", "AP-AAAA-BBBB")
		rr := httptest.NewRecorder()
		h.HandleGetReport(rr, req)

		if rr.Code != http.StatusAccepted {
			t.Fatalf("status = %d, want %d", rr.Code, http.StatusAccepted)
		}
	})

	t.Run("ready", func(t *testing.T) {
		h := newReportHandlerForTest(
			&fakeReportStore{
				getReportBySessionFn: func(context.Context, string) (*Report, error) {
					return &Report{
						SessionCode:             "AP-AAAA-BBBB",
						Status:                  ReportStatusReady,
						ContentEn:               "English report",
						ContentEs:               "Reporte en español",
						AreasOfClarity:          []string{"clarity"},
						AreasOfClarityEs:        []string{"claridad"},
						AreasToDevelopFurther:   []string{"pace"},
						AreasToDevelopFurtherEs: []string{"ritmo"},
						Recommendation:          "continue practice",
						RecommendationEs:        "continúe practicando",
						QuestionCount:           12,
						DurationMinutes:         31,
					}, nil
				},
			},
			&fakeReportSessionProvider{},
		)

		req := withReportAuth(httptest.NewRequest(http.MethodGet, "/api/report/AP-AAAA-BBBB", nil), "AP-AAAA-BBBB")
		req.SetPathValue("code", "AP-AAAA-BBBB")
		rr := httptest.NewRecorder()
		h.HandleGetReport(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
		}
	})

	t.Run("failed", func(t *testing.T) {
		h := newReportHandlerForTest(
			&fakeReportStore{
				getReportBySessionFn: func(context.Context, string) (*Report, error) {
					return &Report{SessionCode: "AP-AAAA-BBBB", Status: ReportStatusFailed}, nil
				},
			},
			&fakeReportSessionProvider{},
		)

		req := withReportAuth(httptest.NewRequest(http.MethodGet, "/api/report/AP-AAAA-BBBB", nil), "AP-AAAA-BBBB")
		req.SetPathValue("code", "AP-AAAA-BBBB")
		rr := httptest.NewRecorder()
		h.HandleGetReport(rr, req)

		if rr.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
		}
	})
}

func TestHandleGenerateReport_TypedResponseContracts(t *testing.T) {
	t.Parallel()

	t.Run("queues_generation", func(t *testing.T) {
		store := &fakeReportStore{
			createReportFn:       func(context.Context, *Report) error { return nil },
			getReportBySessionFn: func(context.Context, string) (*Report, error) { return nil, nil },
		}
		sessions := &fakeReportSessionProvider{
			getSessionByCodeFn: func(context.Context, string) (*SessionInfo, error) {
				return &SessionInfo{SessionCode: "AP-AAAA-BBBB", Status: "completed"}, nil
			},
		}
		h := newReportHandlerForTest(store, sessions)

		req := withReportAuth(httptest.NewRequest(http.MethodPost, "/api/report/AP-AAAA-BBBB/generate", nil), "AP-AAAA-BBBB")
		req.SetPathValue("code", "AP-AAAA-BBBB")
		rr := httptest.NewRecorder()
		h.HandleGenerateReport(rr, req)

		if rr.Code != http.StatusAccepted {
			t.Fatalf("status = %d, want %d", rr.Code, http.StatusAccepted)
		}
	})

	t.Run("returns_ready_when_already_available", func(t *testing.T) {
		store := &fakeReportStore{
			getReportBySessionFn: func(context.Context, string) (*Report, error) {
				return &Report{
					SessionCode: "AP-AAAA-BBBB",
					Status:      ReportStatusReady,
					ContentEn:   "English report",
					ContentEs:   "Reporte en español",
				}, nil
			},
		}
		sessions := &fakeReportSessionProvider{
			getSessionByCodeFn: func(context.Context, string) (*SessionInfo, error) {
				return &SessionInfo{SessionCode: "AP-AAAA-BBBB", Status: "completed"}, nil
			},
		}
		h := newReportHandlerForTest(store, sessions)

		req := withReportAuth(httptest.NewRequest(http.MethodPost, "/api/report/AP-AAAA-BBBB/generate", nil), "AP-AAAA-BBBB")
		req.SetPathValue("code", "AP-AAAA-BBBB")
		rr := httptest.NewRecorder()
		h.HandleGenerateReport(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
		}
	})
}
