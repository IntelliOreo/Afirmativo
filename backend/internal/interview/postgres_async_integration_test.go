package interview

import (
	"context"
	"testing"
	"time"
)

func TestPostgresStoreAsyncAnswerJobClaimAndSucceed(t *testing.T) {
	store, cleanup := newPostgresIntegrationStore(t)
	defer cleanup()

	ctx := context.Background()
	sessionCode := "AP-ASYNC-CLAIM-SUCCEED"
	insertPostgresIntegrationSession(t, store.pool, postgresIntegrationSessionParams{
		SessionCode: sessionCode,
		Status:      "interviewing",
		FlowStep:    FlowStepCriterion,
	})

	job, err := store.UpsertAnswerJob(ctx, UpsertAnswerJobParams{
		SessionCode:     sessionCode,
		ClientRequestID: "req-claim-success",
		TurnID:          "turn-1",
		QuestionText:    "Question text",
		AnswerText:      "Answer text",
	})
	if err != nil {
		t.Fatalf("UpsertAnswerJob() error = %v", err)
	}

	claimed, err := store.ClaimQueuedAnswerJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("ClaimQueuedAnswerJob() error = %v", err)
	}
	if claimed == nil {
		t.Fatal("ClaimQueuedAnswerJob() = nil, want claimed job")
	}
	if claimed.Status != AsyncAnswerJobRunning {
		t.Fatalf("claimed.Status = %q, want %q", claimed.Status, AsyncAnswerJobRunning)
	}
	if claimed.Attempts != 1 {
		t.Fatalf("claimed.Attempts = %d, want 1", claimed.Attempts)
	}
	if claimed.StartedAt == nil {
		t.Fatal("claimed.StartedAt = nil, want non-nil")
	}

	payload := []byte(`{"done":false,"timer_remaining_s":123}`)
	if err := store.MarkAnswerJobSucceeded(ctx, job.ID, payload); err != nil {
		t.Fatalf("MarkAnswerJobSucceeded() error = %v", err)
	}

	got, err := store.GetAnswerJob(ctx, sessionCode, job.ID)
	if err != nil {
		t.Fatalf("GetAnswerJob() error = %v", err)
	}
	if got.Status != AsyncAnswerJobSucceeded {
		t.Fatalf("got.Status = %q, want %q", got.Status, AsyncAnswerJobSucceeded)
	}
	mustEqualPostgresIntegrationJSON(t, got.ResultPayload, payload)
	if got.CompletedAt == nil {
		t.Fatal("got.CompletedAt = nil, want non-nil")
	}

	reclaim, err := store.ClaimQueuedAnswerJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("ClaimQueuedAnswerJob() after success error = %v", err)
	}
	if reclaim != nil {
		t.Fatalf("ClaimQueuedAnswerJob() after success = %#v, want nil", reclaim)
	}
}

func TestPostgresStoreAsyncAnswerJobRequeueStaleRunningJob(t *testing.T) {
	store, cleanup := newPostgresIntegrationStore(t)
	defer cleanup()

	ctx := context.Background()
	sessionCode := "AP-ASYNC-REQUEUE"
	insertPostgresIntegrationSession(t, store.pool, postgresIntegrationSessionParams{
		SessionCode: sessionCode,
		Status:      "interviewing",
		FlowStep:    FlowStepCriterion,
	})

	job, err := store.UpsertAnswerJob(ctx, UpsertAnswerJobParams{
		SessionCode:     sessionCode,
		ClientRequestID: "req-requeue",
		TurnID:          "turn-2",
		QuestionText:    "Question text",
		AnswerText:      "Answer text",
	})
	if err != nil {
		t.Fatalf("UpsertAnswerJob() error = %v", err)
	}

	claimed, err := store.ClaimQueuedAnswerJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("ClaimQueuedAnswerJob() error = %v", err)
	}
	if claimed == nil {
		t.Fatal("ClaimQueuedAnswerJob() = nil, want claimed job")
	}

	staleStartedAt := time.Now().UTC().Add(-10 * time.Minute)
	if _, err := store.pool.Exec(ctx,
		`UPDATE interview_answer_jobs
		 SET started_at = $2,
		     updated_at = $2
		 WHERE id = $1::uuid`,
		job.ID,
		staleStartedAt,
	); err != nil {
		t.Fatalf("force stale started_at error = %v", err)
	}

	requeued, err := store.RequeueStaleRunningAnswerJobs(ctx, time.Now().UTC().Add(-time.Minute))
	if err != nil {
		t.Fatalf("RequeueStaleRunningAnswerJobs() error = %v", err)
	}
	if requeued != 1 {
		t.Fatalf("RequeueStaleRunningAnswerJobs() = %d, want 1", requeued)
	}

	reclaimed, err := store.ClaimQueuedAnswerJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("ClaimQueuedAnswerJob() after requeue error = %v", err)
	}
	if reclaimed == nil {
		t.Fatal("ClaimQueuedAnswerJob() after requeue = nil, want claimed job")
	}
	if reclaimed.Status != AsyncAnswerJobRunning {
		t.Fatalf("reclaimed.Status = %q, want %q", reclaimed.Status, AsyncAnswerJobRunning)
	}
	if reclaimed.Attempts != 2 {
		t.Fatalf("reclaimed.Attempts = %d, want 2", reclaimed.Attempts)
	}
}

func TestPostgresStoreAsyncAnswerJobFailedTerminalStateNotClaimable(t *testing.T) {
	store, cleanup := newPostgresIntegrationStore(t)
	defer cleanup()

	ctx := context.Background()
	sessionCode := "AP-ASYNC-FAILED"
	insertPostgresIntegrationSession(t, store.pool, postgresIntegrationSessionParams{
		SessionCode: sessionCode,
		Status:      "interviewing",
		FlowStep:    FlowStepCriterion,
	})

	job, err := store.UpsertAnswerJob(ctx, UpsertAnswerJobParams{
		SessionCode:     sessionCode,
		ClientRequestID: "req-failed",
		TurnID:          "turn-3",
		QuestionText:    "Question text",
		AnswerText:      "Answer text",
	})
	if err != nil {
		t.Fatalf("UpsertAnswerJob() error = %v", err)
	}

	if err := store.MarkAnswerJobFailed(ctx, MarkAnswerJobFailedParams{
		JobID:        job.ID,
		Status:       AsyncAnswerJobFailed,
		ErrorCode:    "FLOW_INVALID",
		ErrorMessage: "Interview flow is not in a valid state",
	}); err != nil {
		t.Fatalf("MarkAnswerJobFailed() error = %v", err)
	}

	got, err := store.GetAnswerJob(ctx, sessionCode, job.ID)
	if err != nil {
		t.Fatalf("GetAnswerJob() error = %v", err)
	}
	if got.Status != AsyncAnswerJobFailed {
		t.Fatalf("got.Status = %q, want %q", got.Status, AsyncAnswerJobFailed)
	}
	if got.ErrorCode != "FLOW_INVALID" {
		t.Fatalf("got.ErrorCode = %q, want FLOW_INVALID", got.ErrorCode)
	}
	if got.ErrorMessage != "Interview flow is not in a valid state" {
		t.Fatalf("got.ErrorMessage = %q, want terminal failure message", got.ErrorMessage)
	}
	if got.CompletedAt == nil {
		t.Fatal("got.CompletedAt = nil, want non-nil")
	}

	reclaim, err := store.ClaimQueuedAnswerJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("ClaimQueuedAnswerJob() after terminal failure error = %v", err)
	}
	if reclaim != nil {
		t.Fatalf("ClaimQueuedAnswerJob() after terminal failure = %#v, want nil", reclaim)
	}
}

func TestPostgresStoreClaimNextQueuedAnswerJob_ClaimsOldestQueuedJob(t *testing.T) {
	store, cleanup := newPostgresIntegrationStore(t)
	defer cleanup()

	ctx := context.Background()
	sessionCode := "AP-ASYNC-CLAIM-NEXT"
	insertPostgresIntegrationSession(t, store.pool, postgresIntegrationSessionParams{
		SessionCode: sessionCode,
		Status:      "interviewing",
		FlowStep:    FlowStepCriterion,
	})

	firstJob, err := store.UpsertAnswerJob(ctx, UpsertAnswerJobParams{
		SessionCode:     sessionCode,
		ClientRequestID: "req-first",
		LastRequestID:   "request-first",
		TurnID:          "turn-10",
		QuestionText:    "Older question",
		AnswerText:      "Older answer",
	})
	if err != nil {
		t.Fatalf("UpsertAnswerJob(first) error = %v", err)
	}
	secondJob, err := store.UpsertAnswerJob(ctx, UpsertAnswerJobParams{
		SessionCode:     sessionCode,
		ClientRequestID: "req-second",
		LastRequestID:   "request-second",
		TurnID:          "turn-11",
		QuestionText:    "Newer question",
		AnswerText:      "Newer answer",
	})
	if err != nil {
		t.Fatalf("UpsertAnswerJob(second) error = %v", err)
	}

	older := time.Now().UTC().Add(-2 * time.Minute)
	newer := older.Add(time.Minute)
	if _, err := store.pool.Exec(ctx,
		`UPDATE interview_answer_jobs
		    SET created_at = $2,
		        updated_at = $2
		  WHERE id = $1::uuid`,
		firstJob.ID,
		older,
	); err != nil {
		t.Fatalf("force first job ordering error = %v", err)
	}
	if _, err := store.pool.Exec(ctx,
		`UPDATE interview_answer_jobs
		    SET created_at = $2,
		        updated_at = $2
		  WHERE id = $1::uuid`,
		secondJob.ID,
		newer,
	); err != nil {
		t.Fatalf("force second job ordering error = %v", err)
	}

	claimed, err := store.ClaimNextQueuedAnswerJob(ctx)
	if err != nil {
		t.Fatalf("ClaimNextQueuedAnswerJob() error = %v", err)
	}
	if claimed == nil {
		t.Fatal("ClaimNextQueuedAnswerJob() = nil, want claimed job")
	}
	if claimed.ID != firstJob.ID {
		t.Fatalf("claimed.ID = %q, want %q", claimed.ID, firstJob.ID)
	}
	if claimed.Status != AsyncAnswerJobRunning {
		t.Fatalf("claimed.Status = %q, want %q", claimed.Status, AsyncAnswerJobRunning)
	}
	if claimed.Attempts != 1 {
		t.Fatalf("claimed.Attempts = %d, want 1", claimed.Attempts)
	}
	if claimed.StartedAt == nil {
		t.Fatal("claimed.StartedAt = nil, want non-nil")
	}
	if claimed.LastRequestID != "request-first" {
		t.Fatalf("claimed.LastRequestID = %q, want request-first", claimed.LastRequestID)
	}
}

func TestPostgresStoreClaimNextQueuedAnswerJob_ReturnsNilWhenQueueEmpty(t *testing.T) {
	store, cleanup := newPostgresIntegrationStore(t)
	defer cleanup()

	claimed, err := store.ClaimNextQueuedAnswerJob(context.Background())
	if err != nil {
		t.Fatalf("ClaimNextQueuedAnswerJob() error = %v", err)
	}
	if claimed != nil {
		t.Fatalf("ClaimNextQueuedAnswerJob() = %#v, want nil", claimed)
	}
}

func TestPostgresStoreClaimNextQueuedAnswerJob_SkipsLockedJob(t *testing.T) {
	store, cleanup := newPostgresIntegrationStore(t)
	defer cleanup()

	ctx := context.Background()
	sessionCode := "AP-ASYNC-SKIP-LOCKED"
	insertPostgresIntegrationSession(t, store.pool, postgresIntegrationSessionParams{
		SessionCode: sessionCode,
		Status:      "interviewing",
		FlowStep:    FlowStepCriterion,
	})

	firstJob, err := store.UpsertAnswerJob(ctx, UpsertAnswerJobParams{
		SessionCode:     sessionCode,
		ClientRequestID: "req-lock-first",
		TurnID:          "turn-20",
		QuestionText:    "First question",
		AnswerText:      "First answer",
	})
	if err != nil {
		t.Fatalf("UpsertAnswerJob(first) error = %v", err)
	}
	secondJob, err := store.UpsertAnswerJob(ctx, UpsertAnswerJobParams{
		SessionCode:     sessionCode,
		ClientRequestID: "req-lock-second",
		TurnID:          "turn-21",
		QuestionText:    "Second question",
		AnswerText:      "Second answer",
	})
	if err != nil {
		t.Fatalf("UpsertAnswerJob(second) error = %v", err)
	}

	older := time.Now().UTC().Add(-2 * time.Minute)
	newer := older.Add(time.Minute)
	if _, err := store.pool.Exec(ctx,
		`UPDATE interview_answer_jobs
		    SET created_at = $2,
		        updated_at = $2
		  WHERE id = $1::uuid`,
		firstJob.ID,
		older,
	); err != nil {
		t.Fatalf("force first job ordering error = %v", err)
	}
	if _, err := store.pool.Exec(ctx,
		`UPDATE interview_answer_jobs
		    SET created_at = $2,
		        updated_at = $2
		  WHERE id = $1::uuid`,
		secondJob.ID,
		newer,
	); err != nil {
		t.Fatalf("force second job ordering error = %v", err)
	}

	tx, err := store.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`SELECT id
		   FROM interview_answer_jobs
		  WHERE id = $1::uuid
		  FOR UPDATE`,
		firstJob.ID,
	); err != nil {
		t.Fatalf("lock first queued job error = %v", err)
	}

	claimed, err := store.ClaimNextQueuedAnswerJob(ctx)
	if err != nil {
		t.Fatalf("ClaimNextQueuedAnswerJob() error = %v", err)
	}
	if claimed == nil {
		t.Fatal("ClaimNextQueuedAnswerJob() = nil, want claimed job")
	}
	if claimed.ID != secondJob.ID {
		t.Fatalf("claimed.ID = %q, want %q", claimed.ID, secondJob.ID)
	}
}
