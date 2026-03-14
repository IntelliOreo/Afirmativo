package interview

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/afirmativo/backend/internal/session"
)

func TestSelectOpeningQuestion_UsesAIQuestion(t *testing.T) {
	ai := &qaAIClient{
		generateTurnFn: func(_ context.Context, _ *AITurnContext) (*AIResponse, error) {
			return &AIResponse{NextQuestion: "Please explain your claim."}, nil
		},
	}
	svc := newServiceForRecoveryTests(newQAServiceStore(), nil, ai)

	selection, err := svc.selectOpeningQuestion(context.Background(), &AITurnContext{
		CurrentAreaSlug: "protected_ground",
		IsOpeningTurn:   true,
	}, "Fallback protected ground question", nil)
	if err != nil {
		t.Fatalf("selectOpeningQuestion() error = %v", err)
	}
	if selection.questionText != "Please explain your claim." {
		t.Fatalf("questionText = %q, want AI question", selection.questionText)
	}
	if selection.substituted {
		t.Fatalf("substituted = %v, want false", selection.substituted)
	}
	if selection.fallbackReason != openingQuestionFallbackNone {
		t.Fatalf("fallbackReason = %q, want %q", selection.fallbackReason, openingQuestionFallbackNone)
	}
}

func TestSelectOpeningQuestion_RetryExhaustedFallsBack(t *testing.T) {
	originalBackoffs := aiRetryBackoffs
	aiRetryBackoffs = nil
	t.Cleanup(func() {
		aiRetryBackoffs = originalBackoffs
	})

	ai := &qaAIClient{
		generateTurnFn: func(_ context.Context, _ *AITurnContext) (*AIResponse, error) {
			return nil, errors.New("provider unavailable")
		},
	}
	svc := newServiceForRecoveryTests(newQAServiceStore(), nil, ai)

	selection, err := svc.selectOpeningQuestion(context.Background(), &AITurnContext{
		CurrentAreaSlug: "protected_ground",
		IsOpeningTurn:   true,
	}, "Fallback protected ground question", nil)
	if err != nil {
		t.Fatalf("selectOpeningQuestion() error = %v", err)
	}
	if selection.questionText != "Fallback protected ground question" {
		t.Fatalf("questionText = %q, want fallback question", selection.questionText)
	}
	if !selection.substituted {
		t.Fatalf("substituted = %v, want true", selection.substituted)
	}
	if selection.fallbackReason != openingQuestionFallbackRetryExhausted {
		t.Fatalf("fallbackReason = %q, want %q", selection.fallbackReason, openingQuestionFallbackRetryExhausted)
	}
}

func TestSelectOpeningQuestion_EmptyAIQuestionFallsBack(t *testing.T) {
	ai := &qaAIClient{
		generateTurnFn: func(_ context.Context, _ *AITurnContext) (*AIResponse, error) {
			return &AIResponse{NextQuestion: "   "}, nil
		},
	}
	svc := newServiceForRecoveryTests(newQAServiceStore(), nil, ai)

	selection, err := svc.selectOpeningQuestion(context.Background(), &AITurnContext{
		CurrentAreaSlug: "protected_ground",
		IsOpeningTurn:   true,
	}, "Fallback protected ground question", nil)
	if err != nil {
		t.Fatalf("selectOpeningQuestion() error = %v", err)
	}
	if selection.questionText != "Fallback protected ground question" {
		t.Fatalf("questionText = %q, want fallback question", selection.questionText)
	}
	if !selection.substituted {
		t.Fatalf("substituted = %v, want true", selection.substituted)
	}
	if selection.fallbackReason != openingQuestionFallbackEmptyQuestion {
		t.Fatalf("fallbackReason = %q, want %q", selection.fallbackReason, openingQuestionFallbackEmptyQuestion)
	}
}

func TestSelectOpeningQuestion_AbortedRetryPropagatesError(t *testing.T) {
	originalBackoffs := aiRetryBackoffs
	aiRetryBackoffs = []time.Duration{time.Minute}
	t.Cleanup(func() {
		aiRetryBackoffs = originalBackoffs
	})

	ai := &qaAIClient{
		generateTurnFn: func(_ context.Context, _ *AITurnContext) (*AIResponse, error) {
			return nil, errors.New("provider unavailable")
		},
	}
	svc := newServiceForRecoveryTests(newQAServiceStore(), nil, ai)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.selectOpeningQuestion(ctx, &AITurnContext{
		CurrentAreaSlug: "protected_ground",
		IsOpeningTurn:   true,
	}, "Fallback protected ground question", nil)
	if err == nil {
		t.Fatalf("selectOpeningQuestion() error = nil, want non-nil")
	}
	if got := err.Error(); got != "AI retry aborted: context canceled" {
		t.Fatalf("error = %q, want AI retry aborted: context canceled", got)
	}
}

func TestGenerateNextAreaOpeningQuestion_UsesAIQuestion(t *testing.T) {
	ai := &qaAIClient{
		generateTurnFn: func(_ context.Context, _ *AITurnContext) (*AIResponse, error) {
			return &AIResponse{NextQuestion: "Please explain your social group claim."}, nil
		},
	}
	svc := newServiceForRecoveryTests(newQAServiceStore(), nil, ai)

	question, substituted, err := svc.generateNextAreaOpeningQuestion(
		context.Background(),
		"AP-7K9X-M2NF",
		"social_group",
		[]QuestionArea{
			{Area: "protected_ground", Status: AreaStatusComplete},
			{Area: "social_group", Status: AreaStatusPreAddressed, QuestionsCount: 1},
		},
		[]Answer{{QuestionText: "Question one", TranscriptEn: "Answer one"}},
		activeSession("AP-7K9X-M2NF", "en"),
		"en",
		1200,
		nil,
	)
	if err != nil {
		t.Fatalf("generateNextAreaOpeningQuestion() error = %v", err)
	}
	if question != "Please explain your social group claim." {
		t.Fatalf("question = %q, want AI question", question)
	}
	if substituted {
		t.Fatalf("substituted = %v, want false", substituted)
	}
}

func TestGenerateNextAreaOpeningQuestion_RetryExhaustedFallsBack(t *testing.T) {
	originalBackoffs := aiRetryBackoffs
	aiRetryBackoffs = nil
	t.Cleanup(func() {
		aiRetryBackoffs = originalBackoffs
	})

	ai := &qaAIClient{
		generateTurnFn: func(_ context.Context, _ *AITurnContext) (*AIResponse, error) {
			return nil, errors.New("provider unavailable")
		},
	}
	svc := newServiceForRecoveryTests(newQAServiceStore(), nil, ai)

	question, substituted, err := svc.generateNextAreaOpeningQuestion(
		context.Background(),
		"AP-7K9X-M2NF",
		"protected_ground",
		[]QuestionArea{{Area: "protected_ground", Status: AreaStatusInProgress}},
		nil,
		activeSession("AP-7K9X-M2NF", "en"),
		"en",
		1200,
		nil,
	)
	if err != nil {
		t.Fatalf("generateNextAreaOpeningQuestion() error = %v", err)
	}
	if question != "Fallback protected ground question" {
		t.Fatalf("question = %q, want fallback question", question)
	}
	if !substituted {
		t.Fatalf("substituted = %v, want true", substituted)
	}
}

func TestGenerateNextAreaOpeningQuestion_EmptyAIQuestionFallsBack(t *testing.T) {
	ai := &qaAIClient{
		generateTurnFn: func(_ context.Context, _ *AITurnContext) (*AIResponse, error) {
			return &AIResponse{NextQuestion: "   "}, nil
		},
	}
	svc := newServiceForRecoveryTests(newQAServiceStore(), nil, ai)

	question, substituted, err := svc.generateNextAreaOpeningQuestion(
		context.Background(),
		"AP-7K9X-M2NF",
		"protected_ground",
		[]QuestionArea{{Area: "protected_ground", Status: AreaStatusInProgress}},
		nil,
		activeSession("AP-7K9X-M2NF", "en"),
		"en",
		1200,
		nil,
	)
	if err != nil {
		t.Fatalf("generateNextAreaOpeningQuestion() error = %v", err)
	}
	if question != "Fallback protected ground question" {
		t.Fatalf("question = %q, want fallback question", question)
	}
	if !substituted {
		t.Fatalf("substituted = %v, want true", substituted)
	}
}

func TestGenerateNextAreaOpeningQuestion_AbortedRetryPropagatesError(t *testing.T) {
	originalBackoffs := aiRetryBackoffs
	aiRetryBackoffs = []time.Duration{time.Minute}
	t.Cleanup(func() {
		aiRetryBackoffs = originalBackoffs
	})

	ai := &qaAIClient{
		generateTurnFn: func(_ context.Context, _ *AITurnContext) (*AIResponse, error) {
			return nil, errors.New("provider unavailable")
		},
	}
	svc := newServiceForRecoveryTests(newQAServiceStore(), nil, ai)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := svc.generateNextAreaOpeningQuestion(
		ctx,
		"AP-7K9X-M2NF",
		"protected_ground",
		[]QuestionArea{{Area: "protected_ground", Status: AreaStatusInProgress}},
		nil,
		activeSession("AP-7K9X-M2NF", "en"),
		"en",
		1200,
		nil,
	)
	if err == nil {
		t.Fatalf("generateNextAreaOpeningQuestion() error = nil, want non-nil")
	}
	if got := err.Error(); got != "AI retry aborted: context canceled" {
		t.Fatalf("error = %q, want AI retry aborted: context canceled", got)
	}
}

func TestGenerateNextAreaOpeningQuestion_MissingAreaReturnsFallbackWithoutSubstitution(t *testing.T) {
	svc := newServiceForRecoveryTests(newQAServiceStore(), nil, &qaAIClient{})

	question, substituted, err := svc.generateNextAreaOpeningQuestion(
		context.Background(),
		"AP-7K9X-M2NF",
		"social_group",
		[]QuestionArea{{Area: "protected_ground", Status: AreaStatusComplete}},
		nil,
		&session.Session{InterviewBudgetSeconds: 3600},
		"en",
		1200,
		nil,
	)
	if err != nil {
		t.Fatalf("generateNextAreaOpeningQuestion() error = %v", err)
	}
	if question != "Please tell me about social_group." {
		t.Fatalf("question = %q, want fallback question", question)
	}
	if substituted {
		t.Fatalf("substituted = %v, want false", substituted)
	}
}
