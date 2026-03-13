package interview

import (
	"time"

	"github.com/afirmativo/backend/internal/session"
)

func (s *Service) normalizeSubmissionTime(submissionTime time.Time) time.Time {
	if submissionTime.IsZero() {
		submissionTime = s.nowFn()
	}
	return submissionTime.UTC()
}

func (s *Service) calcEffectiveTimeRemaining(sess *session.Session, flowState *FlowState, referenceTime time.Time) int {
	effectiveLapsed := sess.InterviewLapsedSeconds + s.liveCriterionElapsedSeconds(flowState, referenceTime)
	remaining := sess.InterviewBudgetSeconds - effectiveLapsed
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (s *Service) liveCriterionElapsedSeconds(flowState *FlowState, referenceTime time.Time) int {
	if flowState == nil || flowState.Step != FlowStepCriterion || flowState.ActiveQuestion == nil {
		return 0
	}
	if flowState.ActiveQuestion.Question.Kind != QuestionKindCriterion {
		return 0
	}

	elapsed := int(referenceTime.UTC().Sub(flowState.ActiveQuestion.IssuedAt.UTC()).Seconds())
	if elapsed < 0 {
		return 0
	}
	if elapsed > s.settings.AnswerTimeLimitSeconds {
		return s.settings.AnswerTimeLimitSeconds
	}
	return elapsed
}
