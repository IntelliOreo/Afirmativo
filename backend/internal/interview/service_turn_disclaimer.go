package interview

import (
	"context"
	"fmt"
)

func (s *Service) handleDisclaimerTurn(ctx context.Context, sessionCode string, snapshot *turnSnapshot) (*AnswerResult, error) {
	if s.finishIfNoCurrentArea(ctx, sessionCode, snapshot.currentArea, false) {
		return &AnswerResult{Done: true, TimerRemainingS: 0, AnswerSubmitWindowRemainingS: 0}, nil
	}

	readinessTextEs := s.settings.ReadinessQuestion.Es
	readinessTextEn := s.settings.ReadinessQuestion.En
	if snapshot.flowState.QuestionNumber > 1 {
		resumeQuestion := ResumeQuestion(snapshot.currentArea.Area)
		readinessTextEs = resumeQuestion.TextEs
		readinessTextEn = resumeQuestion.TextEn
	}

	nextTurnID, err := newTurnID()
	if err != nil {
		return nil, fmt.Errorf("new turn id: %w", err)
	}
	nextQuestion := ReadinessQuestion(
		snapshot.currentArea.Area,
		readinessTextEs,
		readinessTextEn,
		snapshot.flowState.QuestionNumber,
		nextTurnID,
	)
	issuedQuestion := NewIssuedQuestion(nextQuestion, s.nowFn(), s.settings.AnswerTimeLimitSeconds)

	advanceCtx, advanceCancel := context.WithTimeout(ctx, s.dbTimeout)
	nextFlow, err := s.stateStore.AdvanceNonCriterionStep(advanceCtx, AdvanceNonCriterionStepParams{
		SessionCode:        sessionCode,
		ExpectedTurnID:     snapshot.turnID,
		CurrentStep:        FlowStepDisclaimer,
		NextStep:           FlowStepReadiness,
		EventType:          "disclaimer_ack",
		AnswerText:         snapshot.answerText,
		NextIssuedQuestion: issuedQuestion,
	})
	advanceCancel()
	if err != nil {
		return nil, fmt.Errorf("advance disclaimer step: %w", err)
	}

	return &AnswerResult{
		Done: false,
		NextQuestion: func() *Question {
			if nextFlow.ActiveQuestion != nil {
				return &nextFlow.ActiveQuestion.Question
			}
			return nextQuestion
		}(),
		TimerRemainingS:              snapshot.timeRemainingS,
		AnswerSubmitWindowRemainingS: issuedQuestion.SubmitWindowRemaining(s.nowFn()),
	}, nil
}
