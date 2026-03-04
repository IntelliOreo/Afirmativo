package interview

import (
	"context"
	"errors"
	"testing"
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
