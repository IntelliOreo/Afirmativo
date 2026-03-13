package interview

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/afirmativo/backend/internal/session"
)

func (s *Service) generateNextAreaOpeningQuestion(
	ctx context.Context,
	sessionCode string,
	nextAreaSlug string,
	areas []QuestionArea,
	answers []Answer,
	sess *session.Session,
	preferredLanguage string,
	timeRemainingS int,
	failureRecorder aiRetryFailureRecorder,
) (question string, substituted bool, err error) {
	question = s.fallbackQuestionForArea(nextAreaSlug)

	var nextAreaState *QuestionArea
	for i := range areas {
		if areas[i].Area == nextAreaSlug {
			nextAreaState = &areas[i]
			break
		}
	}
	if nextAreaState == nil {
		return question, false, nil
	}

	nextAreaCfg, nextAreaIndex := s.findAreaConfig(nextAreaSlug)
	openingTurnCtx := s.buildAITurnContext(
		*nextAreaState,
		nextAreaCfg,
		nextAreaIndex,
		answers,
		areas,
		preferredLanguage,
		sess.InterviewBudgetSeconds,
		timeRemainingS,
		true,
		"",
		"",
	)

	slog.Debug("calling AI for next criterion opening question", "session", sessionCode, "area", nextAreaSlug)
	nextAreaAIResult, err := s.callAIWithRetry(ctx, openingTurnCtx, failureRecorder)
	if err != nil {
		if !errors.Is(err, ErrAIRetryExhausted) {
			return "", false, err
		}
		slog.Warn("AI retries exhausted on next criterion opening question, using fallback", "error", err, "area", nextAreaSlug)
		return question, true, nil
	}

	if candidate := strings.TrimSpace(nextAreaAIResult.NextQuestion); candidate != "" {
		return candidate, false, nil
	}

	slog.Warn("AI returned empty next criterion opening question, using fallback", "session", sessionCode, "area", nextAreaSlug)
	return question, true, nil
}

func (s *Service) projectAreasForNextAreaOpening(
	areas []QuestionArea,
	currentArea string,
	decision CriterionTurnDecision,
	preAddressed []PreAddressedArea,
) []QuestionArea {
	projected := make([]QuestionArea, len(areas))
	copy(projected, areas)

	preAddressedEvidence := make(map[string]string, len(preAddressed))
	for _, flag := range preAddressed {
		if strings.TrimSpace(flag.Slug) == "" {
			continue
		}
		preAddressedEvidence[flag.Slug] = flag.Evidence
	}

	for i := range projected {
		switch {
		case projected[i].Area == currentArea:
			if decision.MarkCurrentAs != "" {
				projected[i].Status = decision.MarkCurrentAs
			}
			projected[i].QuestionsCount++
		case projected[i].Status == AreaStatusPending:
			if evidence, ok := preAddressedEvidence[projected[i].Area]; ok {
				projected[i].Status = AreaStatusPreAddressed
				projected[i].PreAddressedEvidence = evidence
			}
		}
	}

	return projected
}

func buildAnswersWithCurrentTurn(answers []Answer, questionText, answerText, preferredLanguage string) []Answer {
	history := make([]Answer, 0, len(answers)+1)
	history = append(history, answers...)

	latest := Answer{QuestionText: questionText}
	if strings.EqualFold(strings.TrimSpace(preferredLanguage), "en") {
		latest.TranscriptEn = answerText
	} else {
		latest.TranscriptEs = answerText
	}
	history = append(history, latest)
	return history
}

func (s *Service) fallbackQuestionForArea(slug string) string {
	areaCfg, _ := s.findAreaConfig(slug)
	nextQuestion := strings.TrimSpace(areaCfg.FallbackQuestion)
	if nextQuestion == "" {
		nextQuestion = fmt.Sprintf("Please tell me about %s.", areaCfg.Label)
	}
	return nextQuestion
}

func (s *Service) fallbackEvaluation(criterionID int) *Evaluation {
	return &Evaluation{
		CurrentCriterion: CurrentCriterion{
			ID:              criterionID,
			Status:          CriterionStatusPartial,
			EvidenceSummary: "Fallback evaluation due to model parsing or provider error.",
			Recommendation:  CriterionRecFollowUp,
		},
		OtherCriteriaAddressed: nil,
	}
}
