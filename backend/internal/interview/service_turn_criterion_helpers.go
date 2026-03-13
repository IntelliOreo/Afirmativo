package interview

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/afirmativo/backend/internal/config"
)

type criterionTurnInputs struct {
	answers   []Answer
	areaCfg   config.AreaConfig
	areaIndex int
	turnCtx   *AITurnContext
}

type criterionTurnEvaluation struct {
	aiResult    *AIResponse
	substituted bool
}

type criterionTurnPlan struct {
	preAddressed     []PreAddressedArea
	decision         CriterionTurnDecision
	projectedAreas   []QuestionArea
	projectedAnswers []Answer
	nextArea         string
}

type criterionTurnNextQuestion struct {
	question       *Question
	issuedQuestion *IssuedQuestion
	substituted    bool
}

func (s *Service) loadCriterionTurnAnswers(ctx context.Context, sessionCode string, snapshot *turnSnapshot) (*criterionTurnInputs, error) {
	answersCtx, answersCancel := context.WithTimeout(ctx, s.dbTimeout)
	answers, err := s.stateStore.GetAnswersBySession(answersCtx, sessionCode)
	answersCancel()
	if err != nil {
		return nil, fmt.Errorf("get answers: %w", err)
	}

	areaCfg, areaIndex := s.findAreaConfig(snapshot.currentArea.Area)
	return &criterionTurnInputs{
		answers:   answers,
		areaCfg:   areaCfg,
		areaIndex: areaIndex,
		turnCtx: s.buildAITurnContext(
			*snapshot.currentArea,
			areaCfg,
			areaIndex,
			answers,
			snapshot.areas,
			snapshot.preferredLanguage,
			snapshot.session.InterviewBudgetSeconds,
			snapshot.timeRemainingS,
			false,
			snapshot.questionText,
			snapshot.answerText,
		),
	}, nil
}

func (s *Service) evaluateCriterionTurn(ctx context.Context, sessionCode string, snapshot *turnSnapshot, inputs *criterionTurnInputs) (*criterionTurnEvaluation, error) {
	slog.Debug("calling AI for criterion turn", "session", sessionCode, "area", snapshot.currentArea.Area)

	evaluation := &criterionTurnEvaluation{}
	aiResult, err := s.callAIWithRetry(ctx, inputs.turnCtx, snapshot.failureRecorder)
	if err != nil {
		if !errors.Is(err, ErrAIRetryExhausted) {
			return nil, err
		}
		evaluation.substituted = true
		slog.Warn("AI retries exhausted, using fallback evaluation", "error", err, "area", snapshot.currentArea.Area)
		aiResult = &AIResponse{
			Evaluation:   s.fallbackEvaluation(inputs.areaCfg.ID),
			NextQuestion: s.fallbackQuestionForArea(snapshot.currentArea.Area),
		}
	}

	if aiResult.Evaluation == nil || aiResult.Evaluation.CurrentCriterion.ID != inputs.areaCfg.ID {
		if aiResult.Evaluation != nil {
			slog.Warn("AI evaluation criterion mismatch, replacing with fallback",
				"session", sessionCode,
				"current_area", snapshot.currentArea.Area,
				"expected_criterion_id", inputs.areaCfg.ID,
				"returned_criterion_id", aiResult.Evaluation.CurrentCriterion.ID,
			)
		}
		aiResult.Evaluation = s.fallbackEvaluation(inputs.areaCfg.ID)
		evaluation.substituted = true
	}

	evaluation.aiResult = aiResult
	return evaluation, nil
}

func (s *Service) planCriterionTransition(snapshot *turnSnapshot, answers []Answer, evaluation *criterionTurnEvaluation) *criterionTurnPlan {
	preAddressed := s.extractPreAddressed(evaluation.aiResult.Evaluation.OtherCriteriaAddressed)
	decision := DecideCriterionTurn(
		evaluation.aiResult.Evaluation.CurrentCriterion,
		snapshot.currentArea.QuestionsCount+1,
		MaxQuestionsPerArea,
	)
	projectedAreas := s.projectAreasForNextAreaOpening(snapshot.areas, snapshot.currentArea.Area, decision, preAddressed)
	projectedAnswers := buildAnswersWithCurrentTurn(answers, snapshot.questionText, snapshot.answerText, snapshot.preferredLanguage)

	return &criterionTurnPlan{
		preAddressed:     preAddressed,
		decision:         decision,
		projectedAreas:   projectedAreas,
		projectedAnswers: projectedAnswers,
		nextArea: DetermineNextAreaAfterCriterionTurn(
			projectedAreas,
			snapshot.currentArea.Area,
			decision,
			preAddressed,
			s.orderedAreaSlugs(),
		),
	}
}

func (s *Service) buildNextCriterionQuestion(
	ctx context.Context,
	sessionCode string,
	snapshot *turnSnapshot,
	plan *criterionTurnPlan,
	evaluation *criterionTurnEvaluation,
) (*criterionTurnNextQuestion, error) {
	result := &criterionTurnNextQuestion{
		substituted: evaluation.substituted,
	}
	if strings.TrimSpace(plan.nextArea) == "" {
		return result, nil
	}

	nextTurnID, err := newTurnID()
	if err != nil {
		return nil, fmt.Errorf("new turn id: %w", err)
	}

	nextQuestionText := strings.TrimSpace(evaluation.aiResult.NextQuestion)
	if plan.decision.Action == CriterionTurnActionNext {
		var nextAreaSubstituted bool
		nextQuestionText, nextAreaSubstituted, err = s.generateNextAreaOpeningQuestion(
			ctx,
			sessionCode,
			plan.nextArea,
			plan.projectedAreas,
			plan.projectedAnswers,
			snapshot.session,
			snapshot.preferredLanguage,
			snapshot.timeRemainingS,
			snapshot.failureRecorder,
		)
		if err != nil {
			return nil, err
		}
		result.substituted = result.substituted || nextAreaSubstituted
	}
	if nextQuestionText == "" {
		result.substituted = true
		slog.Warn("next question is empty after AI processing, using fallback", "session", sessionCode, "area", plan.nextArea)
		nextQuestionText = s.fallbackQuestionForArea(plan.nextArea)
	}

	result.question = &Question{
		TextEs:         nextQuestionText,
		TextEn:         nextQuestionText,
		Area:           plan.nextArea,
		Kind:           QuestionKindCriterion,
		TurnID:         nextTurnID,
		QuestionNumber: snapshot.flowState.QuestionNumber + 1,
		TotalQuestions: EstimatedTotalQuestions,
	}
	result.issuedQuestion = NewIssuedQuestion(result.question, s.nowFn(), s.settings.AnswerTimeLimitSeconds)
	return result, nil
}

func (s *Service) persistCriterionTurn(
	ctx context.Context,
	sessionCode string,
	snapshot *turnSnapshot,
	evaluation *criterionTurnEvaluation,
	plan *criterionTurnPlan,
	nextQuestion *criterionTurnNextQuestion,
) error {
	processCtx, processCancel := context.WithTimeout(ctx, s.dbTimeout)
	defer processCancel()

	_, err := s.stateStore.ProcessCriterionTurn(processCtx, ProcessCriterionTurnParams{
		SessionCode:            sessionCode,
		ExpectedTurnID:         snapshot.turnID,
		CurrentArea:            snapshot.currentArea.Area,
		QuestionText:           snapshot.questionText,
		AnswerText:             snapshot.answerText,
		SubmissionTime:         snapshot.submissionTime,
		PreferredLanguage:      snapshot.preferredLanguage,
		Evaluation:             evaluation.aiResult.Evaluation,
		PreAddressed:           plan.preAddressed,
		Decision:               plan.decision,
		NextArea:               plan.nextArea,
		NextIssuedQuestion:     nextQuestion.issuedQuestion,
		AnswerTimeLimitSeconds: s.settings.AnswerTimeLimitSeconds,
	})
	if err != nil {
		if errors.Is(err, ErrTurnConflict) {
			return ErrTurnConflict
		}
		return fmt.Errorf("process criterion turn: %w", err)
	}
	return nil
}
