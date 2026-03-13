package interview

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
)

func (s *Service) handleReadinessTurn(ctx context.Context, sessionCode string, snapshot *turnSnapshot) (*AnswerResult, error) {
	if s.finishIfNoCurrentArea(ctx, sessionCode, snapshot.currentArea, false) {
		return &AnswerResult{Done: true, TimerRemainingS: 0, AnswerSubmitWindowRemainingS: 0}, nil
	}

	answersCtx, answersCancel := context.WithTimeout(ctx, s.dbTimeout)
	answers, err := s.stateStore.GetAnswersBySession(answersCtx, sessionCode)
	answersCancel()
	if err != nil {
		return nil, fmt.Errorf("get answers: %w", err)
	}

	areaCfg, areaIndex := s.findAreaConfig(snapshot.currentArea.Area)
	nextQuestionText := s.fallbackQuestionForArea(snapshot.currentArea.Area)
	turnCtx := s.buildAITurnContext(
		*snapshot.currentArea,
		areaCfg,
		areaIndex,
		answers,
		snapshot.areas,
		snapshot.preferredLanguage,
		snapshot.session.InterviewBudgetSeconds,
		snapshot.timeRemainingS,
		true,
		"",
		"",
	)

	slog.Debug("calling AI for first criterion question", "session", sessionCode, "area", snapshot.currentArea.Area)
	substituted := false
	aiResult, err := s.callAIWithRetry(ctx, turnCtx, snapshot.failureRecorder)
	if err != nil {
		if !errors.Is(err, ErrAIRetryExhausted) {
			return nil, err
		}
		substituted = true
		slog.Warn("AI retries exhausted on first criterion question, using fallback", "error", err, "area", snapshot.currentArea.Area)
	} else if candidate := strings.TrimSpace(aiResult.NextQuestion); candidate != "" {
		nextQuestionText = candidate
	} else {
		substituted = true
		slog.Warn("AI returned empty first criterion question, using fallback", "session", sessionCode, "area", snapshot.currentArea.Area)
	}

	nextTurnID, err := newTurnID()
	if err != nil {
		return nil, fmt.Errorf("new turn id: %w", err)
	}

	nextQuestion := &Question{
		TextEs:         nextQuestionText,
		TextEn:         nextQuestionText,
		Area:           snapshot.currentArea.Area,
		Kind:           QuestionKindCriterion,
		TurnID:         nextTurnID,
		QuestionNumber: snapshot.flowState.QuestionNumber + 1,
		TotalQuestions: EstimatedTotalQuestions,
	}
	issuedQuestion := NewIssuedQuestion(nextQuestion, s.nowFn(), s.settings.AnswerTimeLimitSeconds)

	advanceCtx, advanceCancel := context.WithTimeout(ctx, s.dbTimeout)
	nextFlow, err := s.stateStore.AdvanceNonCriterionStep(advanceCtx, AdvanceNonCriterionStepParams{
		SessionCode:        sessionCode,
		ExpectedTurnID:     snapshot.turnID,
		CurrentStep:        FlowStepReadiness,
		NextStep:           FlowStepCriterion,
		EventType:          "readiness_ack",
		AnswerText:         snapshot.answerText,
		NextIssuedQuestion: issuedQuestion,
	})
	advanceCancel()
	if err != nil {
		return nil, fmt.Errorf("advance readiness step: %w", err)
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
		Substituted:                  substituted,
	}, nil
}
