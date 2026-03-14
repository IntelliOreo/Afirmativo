package interview

import (
	"context"
	"errors"
	"strings"
)

type openingQuestionFallbackReason string

const (
	openingQuestionFallbackNone           openingQuestionFallbackReason = ""
	openingQuestionFallbackRetryExhausted openingQuestionFallbackReason = "retry_exhausted"
	openingQuestionFallbackEmptyQuestion  openingQuestionFallbackReason = "empty_question"
)

type openingQuestionSelection struct {
	questionText   string
	substituted    bool
	fallbackReason openingQuestionFallbackReason
	fallbackErr    error
}

func (s *Service) selectOpeningQuestion(
	ctx context.Context,
	turnCtx *AITurnContext,
	fallbackQuestion string,
	failureRecorder aiRetryFailureRecorder,
) (*openingQuestionSelection, error) {
	selection := &openingQuestionSelection{
		questionText: fallbackQuestion,
	}

	aiResult, err := s.callAIWithRetry(ctx, turnCtx, failureRecorder)
	if err != nil {
		if !errors.Is(err, ErrAIRetryExhausted) {
			return nil, err
		}
		selection.substituted = true
		selection.fallbackReason = openingQuestionFallbackRetryExhausted
		selection.fallbackErr = err
		return selection, nil
	}

	if candidate := strings.TrimSpace(aiResult.NextQuestion); candidate != "" {
		selection.questionText = candidate
		return selection, nil
	}

	selection.substituted = true
	selection.fallbackReason = openingQuestionFallbackEmptyQuestion
	return selection, nil
}
