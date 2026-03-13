package interview

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
)

func (s *Service) handleReadinessTurn(ctx context.Context, sessionCode string, snapshot *turnSnapshot) (*AnswerResult, error) {
	if result, done := s.finishIfNoCurrentAreaResult(ctx, sessionCode, snapshot.currentArea, false); done {
		return result, nil
	}

	plan, err := s.buildReadinessAdvancePlan(ctx, sessionCode, snapshot)
	if err != nil {
		return nil, err
	}

	return s.advanceNonCriterionTurn(ctx, sessionCode, snapshot, plan)
}

func (s *Service) buildReadinessAdvancePlan(
	ctx context.Context,
	sessionCode string,
	snapshot *turnSnapshot,
) (*nonCriterionAdvancePlan, error) {
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

	return &nonCriterionAdvancePlan{
		opName:      "advance readiness step",
		currentStep: FlowStepReadiness,
		nextStep:    FlowStepCriterion,
		eventType:   "readiness_ack",
		substituted: substituted,
		question: &Question{
			TextEs:         nextQuestionText,
			TextEn:         nextQuestionText,
			Area:           snapshot.currentArea.Area,
			Kind:           QuestionKindCriterion,
			TurnID:         nextTurnID,
			QuestionNumber: snapshot.flowState.QuestionNumber + 1,
			TotalQuestions: EstimatedTotalQuestions,
		},
	}, nil
}
