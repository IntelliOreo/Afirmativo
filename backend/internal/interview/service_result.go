package interview

import "time"

func doneAnswerResult(substituted bool) *AnswerResult {
	return &AnswerResult{
		Done:                         true,
		TimerRemainingS:              0,
		AnswerSubmitWindowRemainingS: 0,
		Substituted:                  substituted,
	}
}

func (s *Service) buildTurnAnswerResult(data issuedQuestionResultData, timeRemainingS int) *AnswerResult {
	return &AnswerResult{
		Done:                         false,
		NextQuestion:                 data.question,
		TimerRemainingS:              timeRemainingS,
		AnswerSubmitWindowRemainingS: data.answerSubmitWindowRemainingS,
		Substituted:                  data.substituted,
	}
}

func resolvedIssuedQuestion(flow *FlowState, fallback *IssuedQuestion) *IssuedQuestion {
	if flow != nil && flow.ActiveQuestion != nil {
		return flow.ActiveQuestion
	}
	return fallback
}

func submitWindowRemaining(issuedQuestion *IssuedQuestion, nowFn func() time.Time) int {
	if issuedQuestion == nil {
		return 0
	}
	return issuedQuestion.SubmitWindowRemaining(nowFn())
}
