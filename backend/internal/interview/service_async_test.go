package interview

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/afirmativo/backend/internal/session"
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
	svc := NewService(sessions, sessions, sessions, &fakeInterviewStore{}, nil, nil, "", "", "", "", AsyncConfig{})

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

func TestStartAsyncAnswerRuntime_ZeroAsyncConfigStillDispatchesRecoveredJobs(t *testing.T) {
	t.Parallel()

	claimed := make(chan string, 1)
	store := &fakeInterviewStore{
		requeueStaleRunningAnswerJobsFn: func(_ context.Context, _ time.Time) (int64, error) {
			return 1, nil
		},
		listQueuedAnswerJobIDsFn: func(_ context.Context, limit int) ([]string, error) {
			if limit <= 0 {
				t.Fatalf("recovery limit = %d, want positive value", limit)
			}
			return []string{"job-a"}, nil
		},
		claimQueuedAnswerJobFn: func(_ context.Context, jobID string) (*AnswerJob, error) {
			select {
			case claimed <- jobID:
			default:
			}
			// nil means "not claimable now", which is enough for this runtime dispatch check.
			return nil, nil
		},
	}

	svc := newInterviewServiceForAsyncTests(store, AsyncConfig{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.StartAsyncAnswerRuntime(ctx)

	select {
	case got := <-claimed:
		if got != "job-a" {
			t.Fatalf("claimed job id = %q, want job-a", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for recovered job dispatch")
	}
}

func TestAsyncConfigWithDefaults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  AsyncConfig
		want AsyncConfig
	}{
		{
			name: "all_zero_values_use_defaults",
			cfg:  AsyncConfig{},
			want: AsyncConfig{
				Workers:       defaultAsyncAnswerWorkers,
				QueueSize:     defaultAsyncAnswerQueueSize,
				RecoveryBatch: defaultAsyncAnswerRecoveryBatch,
				RecoveryEvery: defaultAsyncAnswerRecoveryEvery,
				StaleAfter:    defaultAsyncAnswerStaleRunningAge,
				JobTimeout:    defaultAsyncAnswerJobTimeout,
			},
		},
		{
			name: "partial_config_preserves_supplied_values",
			cfg: AsyncConfig{
				Workers:       9,
				QueueSize:     32,
				RecoveryEvery: 45 * time.Second,
			},
			want: AsyncConfig{
				Workers:       9,
				QueueSize:     32,
				RecoveryBatch: defaultAsyncAnswerRecoveryBatch,
				RecoveryEvery: 45 * time.Second,
				StaleAfter:    defaultAsyncAnswerStaleRunningAge,
				JobTimeout:    defaultAsyncAnswerJobTimeout,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.cfg.withDefaults()
			if got != tc.want {
				t.Fatalf("withDefaults() = %+v, want %+v", got, tc.want)
			}
		})
	}
}
