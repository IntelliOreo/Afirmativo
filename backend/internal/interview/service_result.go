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

func (s *Service) buildTurnAnswerResult(
	issuedQuestion *IssuedQuestion,
	fallbackQuestion *Question,
	timeRemainingS int,
	substituted bool,
) *AnswerResult {
	return &AnswerResult{
		Done:                         false,
		NextQuestion:                 resolveIssuedQuestion(issuedQuestion, fallbackQuestion),
		TimerRemainingS:              timeRemainingS,
		AnswerSubmitWindowRemainingS: submitWindowRemaining(issuedQuestion, s.nowFn),
		Substituted:                  substituted,
	}
}

func resolveIssuedQuestion(issuedQuestion *IssuedQuestion, fallbackQuestion *Question) *Question {
	if issuedQuestion != nil {
		return &issuedQuestion.Question
	}
	return fallbackQuestion
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
