package interview

import (
	"testing"
	"time"
)

func TestLiveCriterionElapsedSeconds_CapsAtAnswerTimeLimit(t *testing.T) {
	t.Parallel()

	svc := newInterviewServiceForAsyncTests(&fakeInterviewStore{})
	issuedAt := time.Date(2026, time.March, 13, 14, 0, 0, 0, time.UTC)

	got := svc.liveCriterionElapsedSeconds(&FlowState{
		Step: FlowStepCriterion,
		ActiveQuestion: &IssuedQuestion{
			Question: Question{Kind: QuestionKindCriterion},
			IssuedAt: issuedAt,
		},
	}, issuedAt.Add(10*time.Minute))

	if got != svc.settings.AnswerTimeLimitSeconds {
		t.Fatalf("liveCriterionElapsedSeconds() = %d, want %d", got, svc.settings.AnswerTimeLimitSeconds)
	}
}

func TestLiveCriterionElapsedSeconds_NonCriterionFlowReturnsZero(t *testing.T) {
	t.Parallel()

	svc := newInterviewServiceForAsyncTests(&fakeInterviewStore{})
	issuedAt := time.Date(2026, time.March, 13, 14, 0, 0, 0, time.UTC)

	got := svc.liveCriterionElapsedSeconds(&FlowState{
		Step: FlowStepReadiness,
		ActiveQuestion: &IssuedQuestion{
			Question: Question{Kind: QuestionKindReadiness},
			IssuedAt: issuedAt,
		},
	}, issuedAt.Add(90*time.Second))

	if got != 0 {
		t.Fatalf("liveCriterionElapsedSeconds() = %d, want 0", got)
	}
}

func TestNormalizeSubmissionTime_UsesNowWhenZero(t *testing.T) {
	t.Parallel()

	svc := newInterviewServiceForAsyncTests(&fakeInterviewStore{})
	expected := time.Date(2026, time.March, 13, 14, 0, 0, 0, time.FixedZone("UTC-5", -5*60*60))
	svc.nowFn = func() time.Time { return expected }

	got := svc.normalizeSubmissionTime(time.Time{})
	want := expected.UTC()

	if !got.Equal(want) {
		t.Fatalf("normalizeSubmissionTime() = %v, want %v", got, want)
	}
}
