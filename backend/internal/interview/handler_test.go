package interview

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func decodeInterviewJSON(t *testing.T, rr *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.Unmarshal(rr.Body.Bytes(), dst); err != nil {
		t.Fatalf("json.Unmarshal() error = %v; body=%s", err, rr.Body.String())
	}
}

func newInterviewHandlerForTest(store Store) *Handler {
	return NewHandler(newInterviewServiceForAsyncTests(store))
}

func TestHandleStart_RejectsInvalidLanguage(t *testing.T) {
	t.Parallel()

	h := newInterviewHandlerForTest(&fakeInterviewStore{})
	req := httptest.NewRequest(http.MethodPost, "/api/interview/start", strings.NewReader(`{"sessionCode":"AP-7K9X-M2NF","language":"fr"}`))
	rr := httptest.NewRecorder()

	h.HandleStart(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
	var got struct {
		Code string `json:"code"`
	}
	decodeInterviewJSON(t, rr, &got)
	if got.Code != "BAD_REQUEST" {
		t.Fatalf("code = %q, want BAD_REQUEST", got.Code)
	}
}

func TestHandleAnswerAsync_ValidationAndIdempotencyErrors(t *testing.T) {
	t.Parallel()

	t.Run("missing_client_request_id", func(t *testing.T) {
		t.Parallel()

		h := newInterviewHandlerForTest(&fakeInterviewStore{})
		req := httptest.NewRequest(http.MethodPost, "/api/interview/answer-async", strings.NewReader(`{
			"sessionCode":"AP-7K9X-M2NF",
			"answerText":"Answer",
			"questionText":"Question",
			"turnId":"turn-1"
		}`))
		rr := httptest.NewRecorder()

		h.HandleAnswerAsync(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
		}
		var got struct {
			Code string `json:"code"`
		}
		decodeInterviewJSON(t, rr, &got)
		if got.Code != "BAD_REQUEST" {
			t.Fatalf("code = %q, want BAD_REQUEST", got.Code)
		}
	})

	t.Run("body_over_10kb", func(t *testing.T) {
		t.Parallel()

		h := newInterviewHandlerForTest(&fakeInterviewStore{})
		answer := strings.Repeat("a", 11*1024)
		body := `{"sessionCode":"AP-7K9X-M2NF","answerText":"` + answer + `","questionText":"Q","turnId":"turn-1","clientRequestId":"req-1"}`
		req := httptest.NewRequest(http.MethodPost, "/api/interview/answer-async", strings.NewReader(body))
		rr := httptest.NewRecorder()

		h.HandleAnswerAsync(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
		}
		var got struct {
			Code string `json:"code"`
		}
		decodeInterviewJSON(t, rr, &got)
		if got.Code != "BAD_REQUEST" {
			t.Fatalf("code = %q, want BAD_REQUEST", got.Code)
		}
	})

	t.Run("idempotency_conflict", func(t *testing.T) {
		t.Parallel()

		store := &fakeInterviewStore{
			upsertAnswerJobFn: func(_ context.Context, _ UpsertAnswerJobParams) (*AnswerJob, error) {
				return &AnswerJob{
					ID:              "job-1",
					SessionCode:     "AP-7K9X-M2NF",
					ClientRequestID: "req-1",
					TurnID:          "turn-1",
					QuestionText:    "Question",
					AnswerText:      "Different stored text",
					Status:          AsyncAnswerJobRunning,
				}, nil
			},
		}
		h := newInterviewHandlerForTest(store)

		req := httptest.NewRequest(http.MethodPost, "/api/interview/answer-async", strings.NewReader(`{
			"sessionCode":"AP-7K9X-M2NF",
			"answerText":"Answer",
			"questionText":"Question",
			"turnId":"turn-1",
			"clientRequestId":"req-1"
		}`))
		rr := httptest.NewRecorder()

		h.HandleAnswerAsync(rr, req)

		if rr.Code != http.StatusConflict {
			t.Fatalf("status = %d, want %d", rr.Code, http.StatusConflict)
		}
		var got struct {
			Code string `json:"code"`
		}
		decodeInterviewJSON(t, rr, &got)
		if got.Code != "IDEMPOTENCY_CONFLICT" {
			t.Fatalf("code = %q, want IDEMPOTENCY_CONFLICT", got.Code)
		}
	})
}

func TestHandleAnswerJobStatus_ValidationAndContract(t *testing.T) {
	t.Parallel()

	t.Run("missing_session_code", func(t *testing.T) {
		t.Parallel()

		h := newInterviewHandlerForTest(&fakeInterviewStore{})
		req := httptest.NewRequest(http.MethodGet, "/api/interview/answer-jobs/job-1", nil)
		req.SetPathValue("jobId", "job-1")
		rr := httptest.NewRecorder()

		h.HandleAnswerJobStatus(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
		}
		var got struct {
			Code string `json:"code"`
		}
		decodeInterviewJSON(t, rr, &got)
		if got.Code != "BAD_REQUEST" {
			t.Fatalf("code = %q, want BAD_REQUEST", got.Code)
		}
	})

	t.Run("job_not_found", func(t *testing.T) {
		t.Parallel()

		h := newInterviewHandlerForTest(&fakeInterviewStore{
			getAnswerJobFn: func(context.Context, string, string) (*AnswerJob, error) {
				return nil, ErrAsyncJobNotFound
			},
		})
		req := httptest.NewRequest(http.MethodGet, "/api/interview/answer-jobs/job-1?sessionCode=AP-7K9X-M2NF", nil)
		req.SetPathValue("jobId", "job-1")
		rr := httptest.NewRecorder()

		h.HandleAnswerJobStatus(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
		}
		var got struct {
			Code string `json:"code"`
		}
		decodeInterviewJSON(t, rr, &got)
		if got.Code != "ASYNC_JOB_NOT_FOUND" {
			t.Fatalf("code = %q, want ASYNC_JOB_NOT_FOUND", got.Code)
		}
	})

	t.Run("succeeded_payload", func(t *testing.T) {
		t.Parallel()

		h := newInterviewHandlerForTest(&fakeInterviewStore{
			getAnswerJobFn: func(_ context.Context, sessionCode, jobID string) (*AnswerJob, error) {
				return &AnswerJob{
					ID:              jobID,
					SessionCode:     sessionCode,
					ClientRequestID: "req-1",
					Status:          AsyncAnswerJobSucceeded,
					ResultPayload: []byte(`{
						"done": false,
						"timerRemainingS": 3540,
						"nextQuestion": {
							"textEs": "¿Cómo se siente hoy?",
							"textEn": "How are you feeling today?",
							"area": "protected_ground",
							"kind": "readiness",
							"turnId": "turn-next",
							"questionNumber": 2,
							"totalQuestions": 25
						}
					}`),
				}, nil
			},
		})
		req := httptest.NewRequest(http.MethodGet, "/api/interview/answer-jobs/job-1?sessionCode=AP-7K9X-M2NF", nil)
		req.SetPathValue("jobId", "job-1")
		rr := httptest.NewRecorder()

		h.HandleAnswerJobStatus(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
		}
		var got struct {
			Status          string `json:"status"`
			Done            bool   `json:"done"`
			TimerRemainingS int    `json:"timerRemainingS"`
			NextQuestion    *struct {
				Kind   string `json:"kind"`
				TurnID string `json:"turnId"`
			} `json:"nextQuestion"`
		}
		decodeInterviewJSON(t, rr, &got)

		if got.Status != string(AsyncAnswerJobSucceeded) {
			t.Fatalf("status = %q, want %q", got.Status, AsyncAnswerJobSucceeded)
		}
		if got.Done {
			t.Fatalf("done = %v, want false", got.Done)
		}
		if got.TimerRemainingS != 3540 {
			t.Fatalf("timerRemainingS = %d, want 3540", got.TimerRemainingS)
		}
		if got.NextQuestion == nil {
			t.Fatalf("nextQuestion = nil, want non-nil")
		}
		if got.NextQuestion.Kind != "readiness" {
			t.Fatalf("nextQuestion.kind = %q, want readiness", got.NextQuestion.Kind)
		}
		if got.NextQuestion.TurnID != "turn-next" {
			t.Fatalf("nextQuestion.turnId = %q, want turn-next", got.NextQuestion.TurnID)
		}
	})
}
