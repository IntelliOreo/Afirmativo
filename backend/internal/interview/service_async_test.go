package interview

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/afirmativo/backend/internal/config"
	"github.com/afirmativo/backend/internal/session"
	"github.com/afirmativo/backend/internal/shared"
)

func TestSubmitAnswerAsync_IdempotencyConflictWhenPayloadDiffers(t *testing.T) {
	t.Parallel()

	store := &fakeInterviewStore{
		upsertAnswerJobFn: func(_ context.Context, _ UpsertAnswerJobParams) (*AnswerJob, error) {
			return &AnswerJob{
				ID:              "job-1",
				SessionCode:     "AP-7K9X-M2NF",
				ClientRequestID: "req-1",
				TurnID:          "turn-1",
				QuestionText:    "Stored question",
				AnswerText:      "Stored answer",
				Status:          AsyncAnswerJobRunning,
			}, nil
		},
	}
	svc := newInterviewServiceForAsyncTests(store)

	_, err := svc.SubmitAnswerAsync(
		context.Background(),
		"AP-7K9X-M2NF",
		"Different answer text",
		"Stored question",
		"turn-1",
		"req-1",
	)
	if !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("SubmitAnswerAsync() error = %v, want ErrIdempotencyConflict", err)
	}
}

func TestSubmitAnswerAsync_PersistsRequestID(t *testing.T) {
	t.Parallel()

	var gotParams UpsertAnswerJobParams
	store := &fakeInterviewStore{
		upsertAnswerJobFn: func(_ context.Context, params UpsertAnswerJobParams) (*AnswerJob, error) {
			gotParams = params
			return &AnswerJob{
				ID:              "job-req",
				SessionCode:     params.SessionCode,
				ClientRequestID: params.ClientRequestID,
				LastRequestID:   params.LastRequestID,
				TurnID:          params.TurnID,
				QuestionText:    params.QuestionText,
				AnswerText:      params.AnswerText,
				Status:          AsyncAnswerJobQueued,
			}, nil
		},
	}
	svc := newInterviewServiceForAsyncTests(store)

	ctx := shared.WithRequestID(context.Background(), "req-interview-1")
	_, err := svc.SubmitAnswerAsync(
		ctx,
		"AP-7K9X-M2NF",
		"Answer text",
		"Question text",
		"turn-1",
		"client-1",
	)
	if err != nil {
		t.Fatalf("SubmitAnswerAsync() error = %v", err)
	}

	if gotParams.LastRequestID != "req-interview-1" {
		t.Fatalf("LastRequestID = %q, want req-interview-1", gotParams.LastRequestID)
	}
}

func TestGetAnswerJobResult_DecodesSucceededPayload(t *testing.T) {
	t.Parallel()

	store := &fakeInterviewStore{
		getAnswerJobFn: func(_ context.Context, sessionCode, jobID string) (*AnswerJob, error) {
			return &AnswerJob{
				ID:              jobID,
				SessionCode:     sessionCode,
				ClientRequestID: "req-1",
				Status:          AsyncAnswerJobSucceeded,
				ResultPayload: []byte(`{
					"done": false,
					"timer_remaining_s": 3540,
					"answer_submit_window_remaining_s": 240,
					"next_question": {
						"text_es": "¿Cómo se siente hoy?",
						"text_en": "How are you feeling today?",
						"area": "protected_ground",
						"kind": "readiness",
						"turn_id": "turn-next",
						"question_number": 2,
						"total_questions": 25
					}
				}`),
			}, nil
		},
	}
	svc := newInterviewServiceForAsyncTests(store)

	got, err := svc.GetAnswerJobResult(context.Background(), "AP-7K9X-M2NF", "job-1")
	if err != nil {
		t.Fatalf("GetAnswerJobResult() error = %v", err)
	}

	if got.Status != AsyncAnswerJobSucceeded {
		t.Fatalf("status = %q, want %q", got.Status, AsyncAnswerJobSucceeded)
	}
	if got.Done {
		t.Fatalf("done = %v, want false", got.Done)
	}
	if got.TimerRemainingS != 3540 {
		t.Fatalf("timerRemainingS = %d, want 3540", got.TimerRemainingS)
	}
	if got.AnswerSubmitWindowRemainingS != 240 {
		t.Fatalf("answerSubmitWindowRemainingS = %d, want 240", got.AnswerSubmitWindowRemainingS)
	}
	if got.NextQuestion == nil {
		t.Fatalf("nextQuestion = nil, want non-nil")
	}
	if got.NextQuestion.Kind != QuestionKindReadiness {
		t.Fatalf("nextQuestion.kind = %q, want %q", got.NextQuestion.Kind, QuestionKindReadiness)
	}
	if got.NextQuestion.TurnID != "turn-next" {
		t.Fatalf("nextQuestion.turnId = %q, want turn-next", got.NextQuestion.TurnID)
	}
}

func TestGetAnswerJobResult_InProgressIgnoresPayload(t *testing.T) {
	t.Parallel()

	store := &fakeInterviewStore{
		getAnswerJobFn: func(_ context.Context, sessionCode, jobID string) (*AnswerJob, error) {
			return &AnswerJob{
				ID:              jobID,
				SessionCode:     sessionCode,
				ClientRequestID: "req-2",
				Status:          AsyncAnswerJobRunning,
				ResultPayload:   []byte(`{"done":true}`),
			}, nil
		},
	}
	svc := newInterviewServiceForAsyncTests(store)

	got, err := svc.GetAnswerJobResult(context.Background(), "AP-7K9X-M2NF", "job-2")
	if err != nil {
		t.Fatalf("GetAnswerJobResult() error = %v", err)
	}

	if got.Status != AsyncAnswerJobRunning {
		t.Fatalf("status = %q, want %q", got.Status, AsyncAnswerJobRunning)
	}
	if got.Done {
		t.Fatalf("done = %v, want false for in-progress jobs", got.Done)
	}
	if got.NextQuestion != nil {
		t.Fatalf("nextQuestion = %#v, want nil for in-progress jobs", got.NextQuestion)
	}
}

func TestSubmitAnswerAsync_AllowsWhitespaceEquivalentPayload(t *testing.T) {
	t.Parallel()

	store := &fakeInterviewStore{
		upsertAnswerJobFn: func(_ context.Context, _ UpsertAnswerJobParams) (*AnswerJob, error) {
			return &AnswerJob{
				ID:              "job-1",
				SessionCode:     "AP-7K9X-M2NF",
				ClientRequestID: "req-1",
				TurnID:          " turn-1 ",
				QuestionText:    " Stored question ",
				AnswerText:      " Stored answer ",
				Status:          AsyncAnswerJobRunning,
			}, nil
		},
	}
	svc := newInterviewServiceForAsyncTests(store)

	result, err := svc.SubmitAnswerAsync(
		context.Background(),
		"AP-7K9X-M2NF",
		"Stored answer",
		"Stored question",
		"turn-1",
		"req-1",
	)
	if err != nil {
		t.Fatalf("SubmitAnswerAsync() error = %v, want nil", err)
	}
	if result.JobID != "job-1" {
		t.Fatalf("jobID = %q, want job-1", result.JobID)
	}
	if result.Status != AsyncAnswerJobRunning {
		t.Fatalf("status = %q, want %q", result.Status, AsyncAnswerJobRunning)
	}
}

func TestSubmitAnswerAsync_RejectsExpiredSession(t *testing.T) {
	t.Parallel()

	expiredAt := time.Now().UTC().Add(-time.Minute)
	sessions := &fakeInterviewSessionStore{
		getSessionByCodeFn: func(context.Context, string) (*session.Session, error) {
			return &session.Session{
				SessionCode: "AP-7K9X-M2NF",
				Status:      "interviewing",
				ExpiresAt:   expiredAt,
			}, nil
		},
	}
	svc := NewService(Deps{
		SessionStarter:   sessions,
		SessionGetter:    sessions,
		SessionCompleter: sessions,
		Store:            &fakeInterviewStore{},
	}, defaultInterviewSettings())

	_, err := svc.SubmitAnswerAsync(
		context.Background(),
		"AP-7K9X-M2NF",
		"Answer text",
		"Question text",
		"turn-1",
		"req-1",
	)
	if !errors.Is(err, session.ErrSessionExpired) {
		t.Fatalf("SubmitAnswerAsync() error = %v, want ErrSessionExpired", err)
	}
}

func TestClassifyAsyncAnswerCompletionOutcome(t *testing.T) {
	t.Parallel()

	outcome, terminal := classifyAsyncAnswerCompletionOutcome(&AnswerResult{Substituted: true})
	if !terminal {
		t.Fatalf("terminal = %v, want true", terminal)
	}
	if outcome.status != AsyncAnswerJobCanceled {
		t.Fatalf("status = %q, want %q", outcome.status, AsyncAnswerJobCanceled)
	}
	if outcome.errorCode != "AI_RETRY_EXHAUSTED" {
		t.Fatalf("errorCode = %q, want AI_RETRY_EXHAUSTED", outcome.errorCode)
	}

	outcome, terminal = classifyAsyncAnswerCompletionOutcome(&AnswerResult{Substituted: false})
	if terminal {
		t.Fatalf("terminal = %v, want false", terminal)
	}
	if outcome != (asyncAnswerTerminalOutcome{}) {
		t.Fatalf("outcome = %#v, want zero value when result is non-terminal", outcome)
	}
}

func TestBuildTurnAnswerResult_PrefersIssuedQuestionAndCarriesFlags(t *testing.T) {
	t.Parallel()

	svc := newInterviewServiceForAsyncTests(&fakeInterviewStore{})
	now := time.Date(2026, time.March, 13, 14, 0, 0, 0, time.UTC)
	svc.nowFn = func() time.Time { return now }

	fallbackQuestion := &Question{
		TextEs:         "Fallback",
		TextEn:         "Fallback",
		Area:           "protected_ground",
		Kind:           QuestionKindCriterion,
		TurnID:         "fallback-turn",
		QuestionNumber: 3,
		TotalQuestions: EstimatedTotalQuestions,
	}
	issuedQuestion := &IssuedQuestion{
		Question: Question{
			TextEs:         "Persisted",
			TextEn:         "Persisted",
			Area:           "protected_ground",
			Kind:           QuestionKindCriterion,
			TurnID:         "persisted-turn",
			QuestionNumber: 4,
			TotalQuestions: EstimatedTotalQuestions,
		},
		IssuedAt:         now,
		AnswerDeadlineAt: now.Add(4 * time.Minute),
	}

	got := svc.buildTurnAnswerResult(svc.issuedQuestionResultData(issuedQuestion, questionIssue{
		question:    fallbackQuestion,
		area:        fallbackQuestion.Area,
		substituted: true,
	}), 1234)
	if got.Done {
		t.Fatalf("done = %v, want false", got.Done)
	}
	if got.NextQuestion == nil {
		t.Fatalf("nextQuestion = nil, want non-nil")
	}
	if got.NextQuestion.TurnID != "persisted-turn" {
		t.Fatalf("nextQuestion.turnID = %q, want persisted-turn", got.NextQuestion.TurnID)
	}
	if got.AnswerSubmitWindowRemainingS != 240 {
		t.Fatalf("answerSubmitWindowRemainingS = %d, want 240", got.AnswerSubmitWindowRemainingS)
	}
	if got.TimerRemainingS != 1234 {
		t.Fatalf("timerRemainingS = %d, want 1234", got.TimerRemainingS)
	}
	if !got.Substituted {
		t.Fatalf("substituted = %v, want true", got.Substituted)
	}
}

func TestProcessTurnForAsyncJob_UsesSubmissionTimeInsteadOfWorkerDelay(t *testing.T) {
	t.Parallel()

	const sessionCode = "AP-7K9X-M2NF"
	submissionTime := time.Date(2026, time.March, 13, 14, 2, 0, 0, time.UTC)
	workerTime := submissionTime.Add(2 * time.Hour)
	issuedAt := submissionTime.Add(-2 * time.Minute)

	store := newQAServiceStore()
	store.getFlowStateFn = func(context.Context, string) (*FlowState, error) {
		return &FlowState{
			Step:           FlowStepCriterion,
			ExpectedTurnID: "turn-criterion",
			QuestionNumber: 3,
			ActiveQuestion: &IssuedQuestion{
				Question: Question{
					TextEs:         "Pregunta actual",
					TextEn:         "Current question",
					Area:           "protected_ground",
					Kind:           QuestionKindCriterion,
					TurnID:         "turn-criterion",
					QuestionNumber: 3,
					TotalQuestions: EstimatedTotalQuestions,
				},
				IssuedAt:         issuedAt,
				AnswerDeadlineAt: issuedAt.Add(5 * time.Minute),
			},
		}, nil
	}
	store.getAreasBySessionFn = func(context.Context, string) ([]QuestionArea, error) {
		return []QuestionArea{{Area: "protected_ground", Status: AreaStatusInProgress, QuestionsCount: 0}}, nil
	}
	store.getInProgressAreaFn = func(context.Context, string) (*QuestionArea, error) {
		return &QuestionArea{Area: "protected_ground", Status: AreaStatusInProgress, QuestionsCount: 0}, nil
	}
	store.getAnswersBySessionFn = func(context.Context, string) ([]Answer, error) {
		return []Answer{}, nil
	}

	var gotAITimeRemaining int
	ai := &qaAIClient{
		generateTurnFn: func(_ context.Context, turnCtx *AITurnContext) (*AIResponse, error) {
			gotAITimeRemaining = turnCtx.TimeRemainingS
			return &AIResponse{
				Evaluation: &Evaluation{
					CurrentCriterion: CurrentCriterion{
						ID:              1,
						Status:          "partially_sufficient",
						Recommendation:  "follow_up",
						EvidenceSummary: "Need more detail",
					},
				},
				NextQuestion: "What happened next?",
			}, nil
		},
	}

	var gotSubmissionTime time.Time
	store.processCriterionTurnFn = func(_ context.Context, params ProcessCriterionTurnParams) (*ProcessCriterionTurnResult, error) {
		gotSubmissionTime = params.SubmissionTime
		return &ProcessCriterionTurnResult{NewCount: 1}, nil
	}

	sessions := &fakeInterviewSessionStore{
		getSessionByCodeFn: func(context.Context, string) (*session.Session, error) {
			return &session.Session{
				SessionCode:            sessionCode,
				PreferredLanguage:      "en",
				Status:                 "interviewing",
				InterviewBudgetSeconds: 2400,
				InterviewLapsedSeconds: 600,
				ExpiresAt:              workerTime.Add(24 * time.Hour),
			}, nil
		},
	}

	svc := newServiceForRecoveryTests(store, sessions, ai)
	svc.nowFn = func() time.Time { return workerTime }

	job := &AnswerJob{
		ID:           "job-1",
		SessionCode:  sessionCode,
		TurnID:       "turn-criterion",
		QuestionText: "Current question",
		AnswerText:   "My answer",
		CreatedAt:    submissionTime,
	}

	result, err := svc.processTurnForAsyncJob(context.Background(), job)
	if err != nil {
		t.Fatalf("processTurnForAsyncJob() error = %v", err)
	}
	if result.Done {
		t.Fatalf("done = %v, want false", result.Done)
	}
	if gotAITimeRemaining != 1680 {
		t.Fatalf("AI timeRemainingS = %d, want 1680 based on submit time instead of worker delay", gotAITimeRemaining)
	}
	if !gotSubmissionTime.Equal(submissionTime) {
		t.Fatalf("ProcessCriterionTurn submissionTime = %v, want %v", gotSubmissionTime, submissionTime)
	}
	if result.TimerRemainingS != 1680 {
		t.Fatalf("timerRemainingS = %d, want 1680 based on submit time instead of worker delay", result.TimerRemainingS)
	}
}

func TestProcessAnswerJob_SessionCompleteFailureStaysRunning(t *testing.T) {
	t.Parallel()

	const sessionCode = "AP-7K9X-M2NF"
	completeErr := errors.New("db connection refused")

	store := newQAServiceStore()
	store.getFlowStateFn = func(context.Context, string) (*FlowState, error) {
		return &FlowState{Step: FlowStepDone}, nil
	}
	store.getAreasBySessionFn = func(context.Context, string) ([]QuestionArea, error) {
		return []QuestionArea{{Area: "protected_ground", Status: AreaStatusComplete}}, nil
	}
	store.getInProgressAreaFn = func(context.Context, string) (*QuestionArea, error) {
		return nil, nil
	}
	store.claimQueuedAnswerJobFn = func(_ context.Context, jobID string) (*AnswerJob, error) {
		return &AnswerJob{
			ID:          jobID,
			SessionCode: sessionCode,
			TurnID:      "turn-1",
			AnswerText:  "answer",
			Status:      AsyncAnswerJobRunning,
			Attempts:    1,
			CreatedAt:   time.Now().UTC(),
		}, nil
	}

	// Track whether any finalize method is called — it shouldn't be.
	markSucceededCalled := false
	markFailedCalled := false
	store.markAnswerJobOKFn = func(context.Context, string, []byte) error {
		markSucceededCalled = true
		return nil
	}
	store.markAnswerJobFailedFn = func(context.Context, MarkAnswerJobFailedParams) error {
		markFailedCalled = true
		return nil
	}

	sessions := &fakeInterviewSessionStore{
		getSessionByCodeFn: func(context.Context, string) (*session.Session, error) {
			return activeSession(sessionCode, "en"), nil
		},
		completeSessionFn: func(context.Context, string) error {
			return completeErr
		},
	}

	svc := newServiceForRecoveryTests(store, sessions, &qaAIClient{})
	claimed, ok := svc.claimAsyncAnswerJob(context.Background(), "job-1", asyncAnswerClaimSourceChannel)
	if !ok {
		t.Fatalf("claimAsyncAnswerJob() = false, want true")
	}
	svc.processClaimedAsyncAnswerJob(context.Background(), claimed)

	if markSucceededCalled {
		t.Fatalf("MarkAnswerJobSucceeded was called; job should stay running for recovery")
	}
	if markFailedCalled {
		t.Fatalf("MarkAnswerJobFailed was called; job should stay running for recovery")
	}
}

func TestProcessAnswerJob_MaxAttemptsExceeded(t *testing.T) {
	t.Parallel()

	const sessionCode = "AP-7K9X-M2NF"

	var gotFailedParams MarkAnswerJobFailedParams
	store := &fakeInterviewStore{
		claimQueuedAnswerJobFn: func(_ context.Context, jobID string) (*AnswerJob, error) {
			return &AnswerJob{
				ID:          jobID,
				SessionCode: sessionCode,
				TurnID:      "turn-1",
				Status:      AsyncAnswerJobRunning,
				Attempts:    6, // > maxAsyncAnswerJobAttempts (5)
				CreatedAt:   time.Now().UTC(),
			}, nil
		},
		markAnswerJobFailedFn: func(_ context.Context, params MarkAnswerJobFailedParams) error {
			gotFailedParams = params
			return nil
		},
	}

	svc := newInterviewServiceForAsyncTests(store)
	claimed, ok := svc.claimAsyncAnswerJob(context.Background(), "job-max", asyncAnswerClaimSourceChannel)
	if !ok {
		t.Fatalf("claimAsyncAnswerJob() = false, want true")
	}
	svc.processClaimedAsyncAnswerJob(context.Background(), claimed)

	if gotFailedParams.JobID != "job-max" {
		t.Fatalf("expected job to be marked failed, got JobID=%q", gotFailedParams.JobID)
	}
	if gotFailedParams.Status != AsyncAnswerJobFailed {
		t.Fatalf("status = %q, want %q", gotFailedParams.Status, AsyncAnswerJobFailed)
	}
	if gotFailedParams.ErrorCode != "SESSION_COMPLETE_FAILED" {
		t.Fatalf("errorCode = %q, want SESSION_COMPLETE_FAILED", gotFailedParams.ErrorCode)
	}
}

func TestEnqueueAsyncAnswerJob_QueueFullReturnsFalse(t *testing.T) {
	t.Parallel()

	svc := newInterviewServiceForAsyncTests(&fakeInterviewStore{}, config.AsyncRuntimeConfig{
		Workers:       1,
		QueueSize:     1,
		RecoveryEvery: time.Hour,
		StaleAfter:    time.Hour,
		JobTimeout:    time.Hour,
	})

	// Fill the queue.
	if !svc.enqueueAsyncAnswerJob("job-fill", "") {
		t.Fatalf("first enqueue should succeed")
	}

	// Queue is full — should return false.
	if svc.enqueueAsyncAnswerJob("job-overflow", "") {
		t.Fatalf("second enqueue should return false when queue is full")
	}
}

func TestProcessAnswerJob_MarkSucceededDBFailureLeavesRunning(t *testing.T) {
	t.Parallel()

	const sessionCode = "AP-7K9X-M2NF"

	store := newQAServiceStore()
	store.getFlowStateFn = func(context.Context, string) (*FlowState, error) {
		return &FlowState{Step: FlowStepDone}, nil
	}
	store.getAreasBySessionFn = func(context.Context, string) ([]QuestionArea, error) {
		return []QuestionArea{{Area: "protected_ground", Status: AreaStatusComplete}}, nil
	}
	store.getInProgressAreaFn = func(context.Context, string) (*QuestionArea, error) {
		return nil, nil
	}
	store.claimQueuedAnswerJobFn = func(_ context.Context, jobID string) (*AnswerJob, error) {
		return &AnswerJob{
			ID:          jobID,
			SessionCode: sessionCode,
			TurnID:      "turn-1",
			AnswerText:  "answer",
			Status:      AsyncAnswerJobRunning,
			Attempts:    1,
			CreatedAt:   time.Now().UTC(),
		}, nil
	}
	store.markAnswerJobOKFn = func(context.Context, string, []byte) error {
		return errors.New("db write failed")
	}

	markFailedCalled := false
	store.markAnswerJobFailedFn = func(context.Context, MarkAnswerJobFailedParams) error {
		markFailedCalled = true
		return nil
	}

	sessions := &fakeInterviewSessionStore{
		getSessionByCodeFn: func(context.Context, string) (*session.Session, error) {
			return activeSession(sessionCode, "en"), nil
		},
	}

	svc := newServiceForRecoveryTests(store, sessions, &qaAIClient{})
	claimed, ok := svc.claimAsyncAnswerJob(context.Background(), "job-mark-fail", asyncAnswerClaimSourceChannel)
	if !ok {
		t.Fatalf("claimAsyncAnswerJob() = false, want true")
	}
	svc.processClaimedAsyncAnswerJob(context.Background(), claimed)

	// When MarkAnswerJobSucceeded fails, the job stays in running (no terminal mark).
	if markFailedCalled {
		t.Fatalf("MarkAnswerJobFailed should not be called when MarkSucceeded fails; job stays running for recovery")
	}
}

func TestStartAsyncAnswerRuntime_ClaimsNextQueuedJobOnIdleFallback(t *testing.T) {
	t.Parallel()

	claimed := make(chan string, 1)
	store := &fakeInterviewStore{
		claimNextQueuedAnswerJobFn: func(_ context.Context) (*AnswerJob, error) {
			select {
			case claimed <- "job-db":
			default:
			}
			return nil, nil
		},
		claimQueuedAnswerJobFn: func(context.Context, string) (*AnswerJob, error) {
			t.Fatalf("channel-hint claim should not run without a hinted job")
			return nil, nil
		},
	}

	svc := newInterviewServiceForAsyncTests(store)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.StartAsyncAnswerRuntime(ctx)

	select {
	case got := <-claimed:
		if got != "job-db" {
			t.Fatalf("claimed job id = %q, want job-db", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for idle fallback claim")
	}
}

func TestRecoverAsyncAnswerJobs_RequeuesOnlyStaleRunning(t *testing.T) {
	t.Parallel()

	requeued := false
	store := &fakeInterviewStore{
		requeueStaleRunningAnswerJobsFn: func(_ context.Context, _ time.Time) (int64, error) {
			requeued = true
			return 1, nil
		},
		claimNextQueuedAnswerJobFn: func(context.Context) (*AnswerJob, error) {
			t.Fatalf("recovery should not claim queued jobs")
			return nil, nil
		},
		claimQueuedAnswerJobFn: func(context.Context, string) (*AnswerJob, error) {
			t.Fatalf("recovery should not issue hinted claims")
			return nil, nil
		},
	}

	svc := newInterviewServiceForAsyncTests(store)
	svc.recoverAsyncAnswerJobs(context.Background())

	if !requeued {
		t.Fatalf("expected stale running jobs to be requeued")
	}
}

func TestClaimAsyncAnswerJob_CanceledContextSkipsDBClaim(t *testing.T) {
	t.Parallel()

	called := false
	store := &fakeInterviewStore{
		claimQueuedAnswerJobFn: func(context.Context, string) (*AnswerJob, error) {
			called = true
			return nil, nil
		},
	}
	svc := newInterviewServiceForAsyncTests(store)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	claimed, ok := svc.claimAsyncAnswerJob(ctx, "job-canceled", asyncAnswerClaimSourceChannel)
	if ok || claimed != nil {
		t.Fatalf("claimAsyncAnswerJob() = (%v, %v), want (nil, false)", claimed, ok)
	}
	if called {
		t.Fatalf("claimAsyncAnswerJob should not hit the store after cancellation")
	}
}

func TestClaimNextAsyncAnswerJob_CanceledContextSkipsDBClaim(t *testing.T) {
	t.Parallel()

	called := false
	store := &fakeInterviewStore{
		claimNextQueuedAnswerJobFn: func(context.Context) (*AnswerJob, error) {
			called = true
			return nil, nil
		},
	}
	svc := newInterviewServiceForAsyncTests(store)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	claimed, ok := svc.claimNextAsyncAnswerJob(ctx)
	if ok || claimed != nil {
		t.Fatalf("claimNextAsyncAnswerJob() = (%v, %v), want (nil, false)", claimed, ok)
	}
	if called {
		t.Fatalf("claimNextAsyncAnswerJob should not hit the store after cancellation")
	}
}

func TestStartAsyncAnswerRuntime_IdleFallbackClaimsQueuedJobAfterQueueOverflow(t *testing.T) {
	t.Parallel()

	claimed := make(chan string, 2)
	store := &fakeInterviewStore{
		claimQueuedAnswerJobFn: func(_ context.Context, jobID string) (*AnswerJob, error) {
			select {
			case claimed <- jobID:
			default:
			}
			return nil, nil
		},
		claimNextQueuedAnswerJobFn: func(_ context.Context) (*AnswerJob, error) {
			select {
			case claimed <- "job-overflow":
			default:
			}
			return nil, nil
		},
	}

	svc := newInterviewServiceForAsyncTests(store, config.AsyncRuntimeConfig{
		Workers:       1,
		QueueSize:     1,
		RecoveryEvery: time.Hour,
		StaleAfter:    time.Hour,
		JobTimeout:    time.Hour,
	})
	if !svc.enqueueAsyncAnswerJob("job-fill", "") {
		t.Fatalf("first enqueue should succeed")
	}
	if svc.enqueueAsyncAnswerJob("job-overflow", "") {
		t.Fatalf("second enqueue should return false when queue is full")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.StartAsyncAnswerRuntime(ctx)

	deadline := time.After(4 * time.Second)
	for {
		select {
		case got := <-claimed:
			if got == "job-overflow" {
				return
			}
		case <-deadline:
			t.Fatalf("timed out waiting for DB fallback claim after queue overflow")
		}
	}
}
