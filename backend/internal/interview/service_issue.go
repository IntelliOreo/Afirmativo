package interview

type questionIssue struct {
	question    *Question
	area        string
	substituted bool
}

type issuedQuestionResultData struct {
	question                     *Question
	area                         string
	answerSubmitWindowRemainingS int
	substituted                  bool
}

func (s *Service) issueQuestion(issue questionIssue) *IssuedQuestion {
	return NewIssuedQuestion(issue.question, s.nowFn(), s.settings.AnswerTimeLimitSeconds)
}

func (s *Service) issuedQuestionResultData(
	issuedQuestion *IssuedQuestion,
	fallback questionIssue,
) issuedQuestionResultData {
	question := fallback.question
	area := fallback.area
	if question != nil && area == "" {
		area = question.Area
	}
	if issuedQuestion != nil {
		question = &issuedQuestion.Question
		area = issuedQuestion.Question.Area
	}

	return issuedQuestionResultData{
		question:                     question,
		area:                         area,
		answerSubmitWindowRemainingS: submitWindowRemaining(issuedQuestion, s.nowFn),
		substituted:                  fallback.substituted,
	}
}

func (s *Service) resolvedIssuedQuestionResultData(
	flow *FlowState,
	fallbackIssuedQuestion *IssuedQuestion,
	fallback questionIssue,
) issuedQuestionResultData {
	return s.issuedQuestionResultData(resolvedIssuedQuestion(flow, fallbackIssuedQuestion), fallback)
}
