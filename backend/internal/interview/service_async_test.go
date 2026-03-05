package interview

import (
	"context"
	"errors"
	"testing"
	"time"
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

func TestConfigureAsyncAnswerRuntime_DefaultsForInvalidInputs(t *testing.T) {
	t.Parallel()

	svc := newInterviewServiceForAsyncTests(&fakeInterviewStore{})

	svc.ConfigureAsyncAnswerRuntime(0, 0, 0, 0, 0, 0)

	if svc.asyncAnswerWorkers != defaultAsyncAnswerWorkers {
		t.Fatalf("workers = %d, want default %d", svc.asyncAnswerWorkers, defaultAsyncAnswerWorkers)
	}
	if cap(svc.asyncAnswerQueue) != defaultAsyncAnswerQueueSize {
		t.Fatalf("queue_size = %d, want default %d", cap(svc.asyncAnswerQueue), defaultAsyncAnswerQueueSize)
	}
	if svc.asyncAnswerRecoveryBatch != defaultAsyncAnswerRecoveryBatch {
		t.Fatalf("recovery_batch = %d, want default %d", svc.asyncAnswerRecoveryBatch, defaultAsyncAnswerRecoveryBatch)
	}
	if svc.asyncAnswerRecoveryEvery != defaultAsyncAnswerRecoveryEvery {
		t.Fatalf("recovery_every = %s, want default %s", svc.asyncAnswerRecoveryEvery, defaultAsyncAnswerRecoveryEvery)
	}
	if svc.asyncAnswerStaleAfter != defaultAsyncAnswerStaleRunningAge {
		t.Fatalf("stale_after = %s, want default %s", svc.asyncAnswerStaleAfter, defaultAsyncAnswerStaleRunningAge)
	}
	if svc.asyncAnswerJobTimeout != defaultAsyncAnswerJobTimeout {
		t.Fatalf("job_timeout = %s, want default %s", svc.asyncAnswerJobTimeout, defaultAsyncAnswerJobTimeout)
	}
}

func TestEnqueueAsyncAnswerJob_BoundedAndTrimmed(t *testing.T) {
	t.Parallel()

	svc := newInterviewServiceForAsyncTests(&fakeInterviewStore{})

	if ok := svc.enqueueAsyncAnswerJob("   "); ok {
		t.Fatalf("empty job id should not enqueue")
	}

	svc.asyncAnswerQueue = nil
	if ok := svc.enqueueAsyncAnswerJob("job-1"); ok {
		t.Fatalf("enqueue should fail when queue is nil")
	}

	svc.asyncAnswerQueue = make(chan string, 1)
	if ok := svc.enqueueAsyncAnswerJob("  job-1  "); !ok {
		t.Fatalf("expected enqueue to succeed")
	}

	got := <-svc.asyncAnswerQueue
	if got != "job-1" {
		t.Fatalf("queued id = %q, want trimmed %q", got, "job-1")
	}

	// Fill queue and verify non-blocking enqueue fails.
	svc.asyncAnswerQueue <- "existing"
	if ok := svc.enqueueAsyncAnswerJob("job-2"); ok {
		t.Fatalf("enqueue should fail when queue is full")
	}
}

func TestRecoverAsyncAnswerJobs_RequeuesAndEnqueuesQueuedJobs(t *testing.T) {
	t.Parallel()

	var (
		gotStaleBefore time.Time
		gotBatchLimit  int
	)
	store := &fakeInterviewStore{
		requeueStaleRunningAnswerJobsFn: func(_ context.Context, staleBefore time.Time) (int64, error) {
			gotStaleBefore = staleBefore
			return 2, nil
		},
		listQueuedAnswerJobIDsFn: func(_ context.Context, limit int) ([]string, error) {
			gotBatchLimit = limit
			return []string{"job-a", "job-b", "job-c"}, nil
		},
	}

	svc := newInterviewServiceForAsyncTests(store)
	svc.ConfigureAsyncAnswerRuntime(1, 2, 2, time.Second, 90*time.Second, time.Minute)

	before := time.Now().UTC()
	svc.recoverAsyncAnswerJobs(context.Background())
	after := time.Now().UTC()

	if gotBatchLimit != 2 {
		t.Fatalf("recovery batch limit = %d, want 2", gotBatchLimit)
	}
	if gotStaleBefore.IsZero() {
		t.Fatalf("expected staleBefore to be passed to requeue call")
	}

	lower := before.Add(-90 * time.Second)
	upper := after.Add(-90 * time.Second)
	if gotStaleBefore.Before(lower) || gotStaleBefore.After(upper) {
		t.Fatalf("staleBefore=%s, want within [%s, %s]", gotStaleBefore, lower, upper)
	}

	var queued []string
	for {
		select {
		case id := <-svc.asyncAnswerQueue:
			queued = append(queued, id)
		default:
			goto done
		}
	}
done:
	if len(queued) != 2 {
		t.Fatalf("queued jobs count = %d, want 2 (queue capacity bound)", len(queued))
	}
	if queued[0] != "job-a" || queued[1] != "job-b" {
		t.Fatalf("queued jobs = %#v, want [job-a job-b] in FIFO order", queued)
	}
}
