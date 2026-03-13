package interview

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/afirmativo/backend/internal/config"
)

type readinessOpeningInputs struct {
	answers          []Answer
	areaCfg          config.AreaConfig
	areaIndex        int
	turnCtx          *AITurnContext
	fallbackQuestion string
}

type readinessOpeningSelection struct {
	questionText string
	substituted  bool
}

func (s *Service) loadReadinessOpeningInputs(
	ctx context.Context,
	sessionCode string,
	snapshot *turnSnapshot,
) (*readinessOpeningInputs, error) {
	answersCtx, answersCancel := context.WithTimeout(ctx, s.dbTimeout)
	answers, err := s.stateStore.GetAnswersBySession(answersCtx, sessionCode)
	answersCancel()
	if err != nil {
		return nil, fmt.Errorf("get answers: %w", err)
	}

	areaCfg, areaIndex := s.findAreaConfig(snapshot.currentArea.Area)
	return &readinessOpeningInputs{
		answers:          answers,
		areaCfg:          areaCfg,
		areaIndex:        areaIndex,
		fallbackQuestion: s.fallbackQuestionForArea(snapshot.currentArea.Area),
		turnCtx: s.buildAITurnContext(
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
		),
	}, nil
}

func (s *Service) selectReadinessOpeningQuestion(
	ctx context.Context,
	sessionCode string,
	snapshot *turnSnapshot,
	inputs *readinessOpeningInputs,
) (*readinessOpeningSelection, error) {
	slog.Debug("calling AI for first criterion question", "session", sessionCode, "area", snapshot.currentArea.Area)

	selection := &readinessOpeningSelection{
		questionText: inputs.fallbackQuestion,
	}

	aiResult, err := s.callAIWithRetry(ctx, inputs.turnCtx, snapshot.failureRecorder)
	if err != nil {
		if !errors.Is(err, ErrAIRetryExhausted) {
			return nil, err
		}
		selection.substituted = true
		slog.Warn("AI retries exhausted on first criterion question, using fallback", "error", err, "area", snapshot.currentArea.Area)
		return selection, nil
	}

	if candidate := strings.TrimSpace(aiResult.NextQuestion); candidate != "" {
		selection.questionText = candidate
		return selection, nil
	}

	selection.substituted = true
	slog.Warn("AI returned empty first criterion question, using fallback", "session", sessionCode, "area", snapshot.currentArea.Area)
	return selection, nil
}

func (s *Service) buildReadinessAdvancePlan(
	snapshot *turnSnapshot,
	selection *readinessOpeningSelection,
) (*nonCriterionAdvancePlan, error) {
	nextTurnID, err := newTurnID()
	if err != nil {
		return nil, fmt.Errorf("new turn id: %w", err)
	}

	return &nonCriterionAdvancePlan{
		opName:      "advance readiness step",
		currentStep: FlowStepReadiness,
		nextStep:    FlowStepCriterion,
		eventType:   "readiness_ack",
		issue: questionIssue{
			question: &Question{
				TextEs:         selection.questionText,
				TextEn:         selection.questionText,
				Area:           snapshot.currentArea.Area,
				Kind:           QuestionKindCriterion,
				TurnID:         nextTurnID,
				QuestionNumber: snapshot.flowState.QuestionNumber + 1,
				TotalQuestions: EstimatedTotalQuestions,
			},
			area:        snapshot.currentArea.Area,
			substituted: selection.substituted,
		},
	}, nil
}
