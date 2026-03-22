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
	getReportBySessionFn         func(ctx context.Context, sessionCode string) (*Report, error)
	createReportFn               func(ctx context.Context, r *Report) error
	setReportQueuedFn            func(ctx context.Context, sessionCode string, resetAttempts bool, lastRequestID string) error
	setReportLastRequestIDFn     func(ctx context.Context, sessionCode, lastRequestID string) error
	claimQueuedReportFn          func(ctx context.Context, sessionCode string) (*Report, error)
	claimNextQueuedReportFn      func(ctx context.Context) (*Report, error)
	requeueStaleRunningReportsFn func(ctx context.Context, staleBefore time.Time) (int64, error)
	markReportReadyFn            func(ctx context.Context, r *Report) error
	markReportFailedFn           func(ctx context.Context, sessionCode, errorCode, errorMessage string) error
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

func (f *fakeReportStore) SetReportQueued(ctx context.Context, sessionCode string, resetAttempts bool, lastRequestID string) error {
	if f.setReportQueuedFn != nil {
		return f.setReportQueuedFn(ctx, sessionCode, resetAttempts, lastRequestID)
	}
	return nil
}

func (f *fakeReportStore) SetReportLastRequestID(ctx context.Context, sessionCode, lastRequestID string) error {
	if f.setReportLastRequestIDFn != nil {
		return f.setReportLastRequestIDFn(ctx, sessionCode, lastRequestID)
	}
	return nil
}

func (f *fakeReportStore) ClaimQueuedReport(ctx context.Context, sessionCode string) (*Report, error) {
	if f.claimQueuedReportFn != nil {
		return f.claimQueuedReportFn(ctx, sessionCode)
	}
	return nil, nil
}

func (f *fakeReportStore) ClaimNextQueuedReport(ctx context.Context) (*Report, error) {
	if f.claimNextQueuedReportFn != nil {
		return f.claimNextQueuedReportFn(ctx)
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

type fakeReportInterviewProvider struct {
	getAreasBySessionFn   func(ctx context.Context, sessionCode string) ([]InterviewAreaSnapshot, error)
	getAnswersBySessionFn func(ctx context.Context, sessionCode string) ([]InterviewAnswerSnapshot, error)
	getAnswerCountFn      func(ctx context.Context, sessionCode string) (int, error)
}

func (f *fakeReportInterviewProvider) GetAreasBySession(ctx context.Context, sessionCode string) ([]InterviewAreaSnapshot, error) {
	if f.getAreasBySessionFn != nil {
		return f.getAreasBySessionFn(ctx, sessionCode)
	}
	return nil, nil
}

func (f *fakeReportInterviewProvider) GetAnswersBySession(ctx context.Context, sessionCode string) ([]InterviewAnswerSnapshot, error) {
	if f.getAnswersBySessionFn != nil {
		return f.getAnswersBySessionFn(ctx, sessionCode)
	}
	return nil, nil
}

func (f *fakeReportInterviewProvider) GetAnswerCount(ctx context.Context, sessionCode string) (int, error) {
	if f.getAnswerCountFn != nil {
		return f.getAnswerCountFn(ctx, sessionCode)
	}
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

func assertContextDeadlineWithin(t *testing.T, ctx context.Context, max time.Duration) {
	t.Helper()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("expected context deadline")
	}

	remaining := time.Until(deadline)
	if remaining <= 0 {
		t.Fatalf("remaining deadline = %v, want > 0", remaining)
	}
	if remaining > max+time.Second {
		t.Fatalf("remaining deadline = %v, want <= %v", remaining, max+time.Second)
	}
}

func defaultReportSettings(areaConfigs []config.AreaConfig) Settings {
	return Settings{
		AreaConfigs: areaConfigs,
		DBTimeout:   5 * time.Second,
		AsyncRuntime: config.AsyncRuntimeConfig{
			Workers:       2,
			QueueSize:     64,
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

func newReportServiceForTest(store Store) *Service {
	return NewService(Deps{
		Store:      store,
		Interviews: &fakeReportInterviewProvider{},
		Sessions:   &fakeReportSessionProvider{},
		AIClient:   &fakeReportAIClient{},
	}, defaultReportSettings(nil))
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

func TestServiceGetReport_UsesDBDeadlines(t *testing.T) {
	t.Parallel()

	const dbTimeout = 2 * time.Second
	store := &fakeReportStore{
		getReportBySessionFn: func(ctx context.Context, sessionCode string) (*Report, error) {
			assertContextDeadlineWithin(t, ctx, dbTimeout)
			if sessionCode != "AP-AAAA-BBBB" {
				t.Fatalf("sessionCode = %q, want AP-AAAA-BBBB", sessionCode)
			}
			return nil, nil
		},
	}
	sessions := &fakeReportSessionProvider{
		getSessionByCodeFn: func(ctx context.Context, sessionCode string) (*SessionInfo, error) {
			assertContextDeadlineWithin(t, ctx, dbTimeout)
			return &SessionInfo{SessionCode: sessionCode, Status: sessionCompleted}, nil
		},
	}
	svc := NewService(Deps{
		Store:      store,
		Interviews: &fakeReportInterviewProvider{},
		Sessions:   sessions,
		AIClient:   &fakeReportAIClient{},
	}, Settings{
		AreaConfigs: nil,
		DBTimeout:   dbTimeout,
		AsyncRuntime: config.AsyncRuntimeConfig{
			Workers:       2,
			QueueSize:     64,
			RecoveryEvery: 10 * time.Second,
			StaleAfter:    3 * time.Minute,
			JobTimeout:    3 * time.Minute,
		},
	})

	_, err := svc.GetReport(context.Background(), "AP-AAAA-BBBB")
	if !errors.Is(err, ErrReportNotStarted) {
		t.Fatalf("GetReport() error = %v, want ErrReportNotStarted", err)
	}
}

func TestServiceGenerateReportAsync_QueuesFailedReport(t *testing.T) {
	t.Parallel()

	const dbTimeout = 2 * time.Second
	queued := false
	store := &fakeReportStore{
		getReportBySessionFn: func(ctx context.Context, sessionCode string) (*Report, error) {
			assertContextDeadlineWithin(t, ctx, dbTimeout)
			return &Report{SessionCode: sessionCode, Status: ReportStatusFailed}, nil
		},
		setReportQueuedFn: func(ctx context.Context, sessionCode string, resetAttempts bool, lastRequestID string) error {
			assertContextDeadlineWithin(t, ctx, dbTimeout)
			if sessionCode != "AP-AAAA-BBBB" {
				t.Fatalf("sessionCode = %q, want AP-AAAA-BBBB", sessionCode)
			}
			if !resetAttempts {
				t.Fatalf("resetAttempts = %v, want true", resetAttempts)
			}
			if lastRequestID != "" {
				t.Fatalf("lastRequestID = %q, want empty without request context", lastRequestID)
			}
			queued = true
			return nil
		},
	}
	sessions := &fakeReportSessionProvider{
		getSessionByCodeFn: func(ctx context.Context, sessionCode string) (*SessionInfo, error) {
			assertContextDeadlineWithin(t, ctx, dbTimeout)
			return &SessionInfo{SessionCode: sessionCode, Status: sessionCompleted}, nil
		},
	}
	svc := NewService(Deps{
		Store:      store,
		Interviews: &fakeReportInterviewProvider{},
		Sessions:   sessions,
		AIClient:   &fakeReportAIClient{},
	}, Settings{
		AreaConfigs: nil,
		DBTimeout:   dbTimeout,
		AsyncRuntime: config.AsyncRuntimeConfig{
			Workers:       2,
			QueueSize:     64,
			RecoveryEvery: 10 * time.Second,
			StaleAfter:    3 * time.Minute,
			JobTimeout:    3 * time.Minute,
		},
	})

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

func TestServiceGenerateReportAsync_UpdatesLastRequestIDForQueuedReport(t *testing.T) {
	t.Parallel()

	updated := false
	store := &fakeReportStore{
		getReportBySessionFn: func(context.Context, string) (*Report, error) {
			return &Report{SessionCode: "AP-AAAA-BBBB", Status: ReportStatusQueued}, nil
		},
		setReportLastRequestIDFn: func(_ context.Context, sessionCode, lastRequestID string) error {
			if sessionCode != "AP-AAAA-BBBB" {
				t.Fatalf("sessionCode = %q, want AP-AAAA-BBBB", sessionCode)
			}
			if lastRequestID != "req-report-1" {
				t.Fatalf("lastRequestID = %q, want req-report-1", lastRequestID)
			}
			updated = true
			return nil
		},
	}
	sessions := &fakeReportSessionProvider{
		getSessionByCodeFn: func(context.Context, string) (*SessionInfo, error) {
			return &SessionInfo{SessionCode: "AP-AAAA-BBBB", Status: sessionCompleted}, nil
		},
	}
	svc := NewService(Deps{
		Store:      store,
		Interviews: &fakeReportInterviewProvider{},
		Sessions:   sessions,
		AIClient:   &fakeReportAIClient{},
	}, defaultReportSettings(nil))

	ctx := shared.WithRequestID(context.Background(), "req-report-1")
	report, err := svc.GenerateReportAsync(ctx, "AP-AAAA-BBBB")
	if err != nil {
		t.Fatalf("GenerateReportAsync() error = %v", err)
	}
	if !updated {
		t.Fatal("expected SetReportLastRequestID to be called")
	}
	if report.LastRequestID != "req-report-1" {
		t.Fatalf("report.LastRequestID = %q, want req-report-1", report.LastRequestID)
	}
}

func TestServiceGenerateReportAsync_PersistsLastRequestIDOnCreate(t *testing.T) {
	t.Parallel()

	created := false
	store := &fakeReportStore{
		getReportBySessionFn: func(context.Context, string) (*Report, error) { return nil, nil },
		createReportFn: func(_ context.Context, r *Report) error {
			created = true
			if r.LastRequestID != "req-report-create" {
				t.Fatalf("r.LastRequestID = %q, want req-report-create", r.LastRequestID)
			}
			return nil
		},
	}
	sessions := &fakeReportSessionProvider{
		getSessionByCodeFn: func(context.Context, string) (*SessionInfo, error) {
			return &SessionInfo{SessionCode: "AP-AAAA-BBBB", Status: sessionCompleted}, nil
		},
	}
	svc := NewService(Deps{
		Store:      store,
		Interviews: &fakeReportInterviewProvider{},
		Sessions:   sessions,
		AIClient:   &fakeReportAIClient{},
	}, defaultReportSettings(nil))

	ctx := shared.WithRequestID(context.Background(), "req-report-create")
	report, err := svc.GenerateReportAsync(ctx, "AP-AAAA-BBBB")
	if err != nil {
		t.Fatalf("GenerateReportAsync() error = %v", err)
	}
	if !created {
		t.Fatal("expected CreateReport to be called")
	}
	if report.LastRequestID != "req-report-create" {
		t.Fatalf("report.LastRequestID = %q, want req-report-create", report.LastRequestID)
	}
}

func TestServiceBuildReportInput_UsesDBDeadlines(t *testing.T) {
	t.Parallel()

	const dbTimeout = 2 * time.Second
	interviews := &fakeReportInterviewProvider{
		getAreasBySessionFn: func(ctx context.Context, sessionCode string) ([]InterviewAreaSnapshot, error) {
			assertContextDeadlineWithin(t, ctx, dbTimeout)
			return []InterviewAreaSnapshot{{Area: "area_1", Status: "complete"}}, nil
		},
		getAnswersBySessionFn: func(ctx context.Context, sessionCode string) ([]InterviewAnswerSnapshot, error) {
			assertContextDeadlineWithin(t, ctx, dbTimeout)
			return []InterviewAnswerSnapshot{{Area: "open_floor", QuestionText: "Q?", TranscriptEn: "A"}}, nil
		},
		getAnswerCountFn: func(ctx context.Context, sessionCode string) (int, error) {
			assertContextDeadlineWithin(t, ctx, dbTimeout)
			return 1, nil
		},
	}
	svc := NewService(Deps{
		Store:      &fakeReportStore{},
		Interviews: interviews,
		Sessions:   &fakeReportSessionProvider{},
		AIClient:   &fakeReportAIClient{},
	}, Settings{
		AreaConfigs: []config.AreaConfig{{Slug: "area_1", Label: "Area 1"}},
		DBTimeout:   dbTimeout,
		AsyncRuntime: config.AsyncRuntimeConfig{
			Workers:       2,
			QueueSize:     64,
			RecoveryEvery: 10 * time.Second,
			StaleAfter:    3 * time.Minute,
			JobTimeout:    3 * time.Minute,
		},
	})

	input, err := svc.buildReportInput(context.Background(), "AP-AAAA-BBBB", &SessionInfo{
		SessionCode:       "AP-AAAA-BBBB",
		Status:            sessionCompleted,
		PreferredLanguage: "en",
	})
	if err != nil {
		t.Fatalf("buildReportInput() error = %v", err)
	}
	if input.answerCount != 1 {
		t.Fatalf("answerCount = %d, want 1", input.answerCount)
	}
}

func TestStartAsyncReportRuntime_ClaimsNextQueuedReportOnIdleFallback(t *testing.T) {
	t.Parallel()

	claimed := make(chan string, 1)
	store := &fakeReportStore{
		claimNextQueuedReportFn: func(_ context.Context) (*Report, error) {
			select {
			case claimed <- "AP-DB-FALLBACK":
			default:
			}
			return nil, nil
		},
		claimQueuedReportFn: func(context.Context, string) (*Report, error) {
			t.Fatalf("channel-hint claim should not run without a hinted report")
			return nil, nil
		},
	}

	svc := newReportServiceForTest(store)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.StartAsyncRuntime(ctx)

	select {
	case got := <-claimed:
		if got != "AP-DB-FALLBACK" {
			t.Fatalf("claimed session code = %q, want AP-DB-FALLBACK", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for idle fallback claim")
	}
}

func TestRecoverAsyncReports_RequeuesOnlyStaleRunning(t *testing.T) {
	t.Parallel()

	requeued := false
	store := &fakeReportStore{
		requeueStaleRunningReportsFn: func(_ context.Context, _ time.Time) (int64, error) {
			requeued = true
			return 1, nil
		},
		claimNextQueuedReportFn: func(context.Context) (*Report, error) {
			t.Fatalf("recovery should not claim queued reports")
			return nil, nil
		},
		claimQueuedReportFn: func(context.Context, string) (*Report, error) {
			t.Fatalf("recovery should not issue hinted claims")
			return nil, nil
		},
	}

	svc := newReportServiceForTest(store)
	svc.recoverAsyncReports(context.Background())

	if !requeued {
		t.Fatalf("expected stale running reports to be requeued")
	}
}

func TestClaimQueuedReportByHint_CanceledContextSkipsDBClaim(t *testing.T) {
	t.Parallel()

	called := false
	store := &fakeReportStore{
		claimQueuedReportFn: func(context.Context, string) (*Report, error) {
			called = true
			return nil, nil
		},
	}
	svc := newReportServiceForTest(store)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	claimed, ok := svc.claimQueuedReportByHint(ctx, "AP-CANCELED")
	if ok || claimed != nil {
		t.Fatalf("claimQueuedReportByHint() = (%v, %v), want (nil, false)", claimed, ok)
	}
	if called {
		t.Fatalf("claimQueuedReportByHint should not hit the store after cancellation")
	}
}

func TestClaimNextQueuedReport_CanceledContextSkipsDBClaim(t *testing.T) {
	t.Parallel()

	called := false
	store := &fakeReportStore{
		claimNextQueuedReportFn: func(context.Context) (*Report, error) {
			called = true
			return nil, nil
		},
	}
	svc := newReportServiceForTest(store)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	claimed, ok := svc.claimNextQueuedReport(ctx)
	if ok || claimed != nil {
		t.Fatalf("claimNextQueuedReport() = (%v, %v), want (nil, false)", claimed, ok)
	}
	if called {
		t.Fatalf("claimNextQueuedReport should not hit the store after cancellation")
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
