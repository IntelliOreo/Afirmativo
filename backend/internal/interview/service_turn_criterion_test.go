package interview

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/afirmativo/backend/internal/config"
)

func TestEvaluateCriterionTurn_RetryExhaustedUsesFallbackEvaluation(t *testing.T) {
	originalBackoffs := aiRetryBackoffs
	aiRetryBackoffs = nil
	t.Cleanup(func() {
		aiRetryBackoffs = originalBackoffs
	})

	ai := &qaAIClient{
		generateTurnFn: func(context.Context, *AITurnContext) (*AIResponse, error) {
			return nil, errors.New("provider unavailable")
		},
	}
	svc := newServiceForControlFlowTests(newQAServiceStore(), nil, ai)

	evaluation, err := svc.evaluateCriterionTurn(context.Background(), "AP-7K9X-M2NF", &turnSnapshot{
		currentArea: &QuestionArea{Area: "protected_ground"},
	}, &criterionTurnInputs{
		areaCfg: config.AreaConfig{ID: 1, Slug: "protected_ground"},
		turnCtx: &AITurnContext{CurrentAreaSlug: "protected_ground"},
	})
	if err != nil {
		t.Fatalf("evaluateCriterionTurn() error = %v", err)
	}
	if !evaluation.substituted {
		t.Fatalf("substituted = %v, want true", evaluation.substituted)
	}
	if evaluation.aiResult == nil || evaluation.aiResult.Evaluation == nil {
		t.Fatalf("aiResult.evaluation = %#v, want non-nil", evaluation.aiResult)
	}
	if evaluation.aiResult.Evaluation.CurrentCriterion.ID != 1 {
		t.Fatalf("criterion id = %d, want 1", evaluation.aiResult.Evaluation.CurrentCriterion.ID)
	}
	if evaluation.aiResult.Evaluation.CurrentCriterion.Status != CriterionStatusPartial {
		t.Fatalf("criterion status = %q, want %q", evaluation.aiResult.Evaluation.CurrentCriterion.Status, CriterionStatusPartial)
	}
	if evaluation.aiResult.Evaluation.CurrentCriterion.Recommendation != CriterionRecFollowUp {
		t.Fatalf("recommendation = %q, want %q", evaluation.aiResult.Evaluation.CurrentCriterion.Recommendation, CriterionRecFollowUp)
	}
	if evaluation.aiResult.NextQuestion != "Fallback protected ground question" {
		t.Fatalf("nextQuestion = %q, want fallback question", evaluation.aiResult.NextQuestion)
	}
}

func TestEvaluateCriterionTurn_CriterionMismatchUsesFallbackEvaluation(t *testing.T) {
	ai := &qaAIClient{
		generateTurnFn: func(context.Context, *AITurnContext) (*AIResponse, error) {
			return &AIResponse{
				Evaluation: &Evaluation{
					CurrentCriterion: CurrentCriterion{
						ID:              99,
						Status:          CriterionStatusSufficient,
						EvidenceSummary: "Wrong criterion",
						Recommendation:  CriterionRecMoveOn,
					},
				},
				NextQuestion: "AI follow-up question",
			}, nil
		},
	}
	svc := newServiceForControlFlowTests(newQAServiceStore(), nil, ai)

	evaluation, err := svc.evaluateCriterionTurn(context.Background(), "AP-7K9X-M2NF", &turnSnapshot{
		currentArea: &QuestionArea{Area: "protected_ground"},
	}, &criterionTurnInputs{
		areaCfg: config.AreaConfig{ID: 1, Slug: "protected_ground"},
		turnCtx: &AITurnContext{CurrentAreaSlug: "protected_ground"},
	})
	if err != nil {
		t.Fatalf("evaluateCriterionTurn() error = %v", err)
	}
	if !evaluation.substituted {
		t.Fatalf("substituted = %v, want true", evaluation.substituted)
	}
	if evaluation.aiResult == nil || evaluation.aiResult.Evaluation == nil {
		t.Fatalf("aiResult.evaluation = %#v, want non-nil", evaluation.aiResult)
	}
	if evaluation.aiResult.Evaluation.CurrentCriterion.ID != 1 {
		t.Fatalf("criterion id = %d, want 1", evaluation.aiResult.Evaluation.CurrentCriterion.ID)
	}
	if evaluation.aiResult.NextQuestion != "AI follow-up question" {
		t.Fatalf("nextQuestion = %q, want original AI next question", evaluation.aiResult.NextQuestion)
	}
}

func TestPlanCriterionTransition_StayAndNext(t *testing.T) {
	svc := newServiceForControlFlowTests(newQAServiceStore(), nil, &qaAIClient{})
	snapshot := &turnSnapshot{
		currentArea:       &QuestionArea{Area: "protected_ground", QuestionsCount: 1},
		areas:             []QuestionArea{{Area: "protected_ground", Status: AreaStatusInProgress, QuestionsCount: 1}, {Area: "social_group", Status: AreaStatusPending}},
		questionText:      "Question",
		answerText:        "Answer",
		preferredLanguage: "en",
	}
	answers := []Answer{{QuestionText: "Earlier question", TranscriptEn: "Earlier answer"}}

	stayPlan := svc.planCriterionTransition(snapshot, answers, &criterionTurnEvaluation{
		aiResult: &AIResponse{
			Evaluation: &Evaluation{
				CurrentCriterion: CurrentCriterion{
					ID:             1,
					Status:         CriterionStatusPartial,
					Recommendation: CriterionRecFollowUp,
				},
			},
		},
	})
	if stayPlan.decision.Action != CriterionTurnActionStay {
		t.Fatalf("stay decision.action = %q, want %q", stayPlan.decision.Action, CriterionTurnActionStay)
	}
	if stayPlan.nextArea != "protected_ground" {
		t.Fatalf("stay nextArea = %q, want protected_ground", stayPlan.nextArea)
	}
	if len(stayPlan.projectedAnswers) != 2 {
		t.Fatalf("stay projectedAnswers length = %d, want 2", len(stayPlan.projectedAnswers))
	}

	nextPlan := svc.planCriterionTransition(snapshot, answers, &criterionTurnEvaluation{
		aiResult: &AIResponse{
			Evaluation: &Evaluation{
				CurrentCriterion: CurrentCriterion{
					ID:             1,
					Status:         CriterionStatusSufficient,
					Recommendation: CriterionRecMoveOn,
				},
			},
		},
	})
	if nextPlan.decision.Action != CriterionTurnActionNext {
		t.Fatalf("next decision.action = %q, want %q", nextPlan.decision.Action, CriterionTurnActionNext)
	}
	if nextPlan.nextArea != "social_group" {
		t.Fatalf("next nextArea = %q, want social_group", nextPlan.nextArea)
	}
	if nextPlan.projectedAreas[0].Status != AreaStatusComplete {
		t.Fatalf("projected current area status = %q, want %q", nextPlan.projectedAreas[0].Status, AreaStatusComplete)
	}
}

func TestBuildNextCriterionQuestion_EmptyFollowUpFallsBack(t *testing.T) {
	svc := newServiceForControlFlowTests(newQAServiceStore(), nil, &qaAIClient{})

	nextQuestion, err := svc.buildNextCriterionQuestion(context.Background(), "AP-7K9X-M2NF", &turnSnapshot{
		currentArea:    &QuestionArea{Area: "protected_ground"},
		flowState:      &FlowState{QuestionNumber: 2},
		session:        activeSession("AP-7K9X-M2NF", "en"),
		timeRemainingS: 1200,
	}, &criterionTurnPlan{
		decision: CriterionTurnDecision{Action: CriterionTurnActionStay},
		nextArea: "protected_ground",
	}, &criterionTurnEvaluation{
		aiResult: &AIResponse{NextQuestion: "   "},
	})
	if err != nil {
		t.Fatalf("buildNextCriterionQuestion() error = %v", err)
	}
	if nextQuestion.question == nil {
		t.Fatalf("question = nil, want non-nil")
	}
	if nextQuestion.question.TextEn != "Fallback protected ground question" {
		t.Fatalf("question.textEn = %q, want fallback question", nextQuestion.question.TextEn)
	}
	if !nextQuestion.substituted {
		t.Fatalf("substituted = %v, want true", nextQuestion.substituted)
	}
	if nextQuestion.issuedQuestion == nil {
		t.Fatalf("issuedQuestion = nil, want non-nil")
	}
}

func TestBuildNextCriterionQuestion_NextAreaUsesProjectedState(t *testing.T) {
	var openingTurnCtx *AITurnContext
	ai := &qaAIClient{
		generateTurnFn: func(_ context.Context, turnCtx *AITurnContext) (*AIResponse, error) {
			openingTurnCtx = turnCtx
			return &AIResponse{NextQuestion: "Please explain your social group claim."}, nil
		},
	}
	svc := newServiceForControlFlowTests(newQAServiceStore(), nil, ai)

	nextQuestion, err := svc.buildNextCriterionQuestion(context.Background(), "AP-7K9X-M2NF", &turnSnapshot{
		flowState:         &FlowState{QuestionNumber: 4},
		session:           activeSession("AP-7K9X-M2NF", "en"),
		preferredLanguage: "en",
		timeRemainingS:    1200,
	}, &criterionTurnPlan{
		decision: CriterionTurnDecision{Action: CriterionTurnActionNext},
		nextArea: "social_group",
		projectedAreas: []QuestionArea{
			{Area: "protected_ground", Status: AreaStatusComplete, QuestionsCount: 2},
			{Area: "social_group", Status: AreaStatusPreAddressed, QuestionsCount: 1},
		},
		projectedAnswers: []Answer{
			{QuestionText: "Question one", TranscriptEn: "Answer one"},
			{QuestionText: "Question two", TranscriptEn: "Answer two"},
		},
	}, &criterionTurnEvaluation{
		aiResult: &AIResponse{NextQuestion: "Unused next question"},
	})
	if err != nil {
		t.Fatalf("buildNextCriterionQuestion() error = %v", err)
	}
	if nextQuestion.question == nil {
		t.Fatalf("question = nil, want non-nil")
	}
	if nextQuestion.question.TextEn != "Please explain your social group claim." {
		t.Fatalf("question.textEn = %q, want AI opening question", nextQuestion.question.TextEn)
	}
	if openingTurnCtx == nil {
		t.Fatalf("openingTurnCtx = nil, want captured context")
	}
	if openingTurnCtx.CurrentAreaSlug != "social_group" {
		t.Fatalf("openingTurnCtx.currentAreaSlug = %q, want social_group", openingTurnCtx.CurrentAreaSlug)
	}
	if !openingTurnCtx.IsOpeningTurn {
		t.Fatalf("openingTurnCtx.isOpeningTurn = %v, want true", openingTurnCtx.IsOpeningTurn)
	}
	if len(openingTurnCtx.HistoryTurns) != 2 {
		t.Fatalf("openingTurnCtx.historyTurns length = %d, want 2", len(openingTurnCtx.HistoryTurns))
	}
	if openingTurnCtx.HistoryTurns[0].AnswerText != "Answer one" {
		t.Fatalf("openingTurnCtx.historyTurns[0].answerText = %q, want Answer one", openingTurnCtx.HistoryTurns[0].AnswerText)
	}
	if openingTurnCtx.HistoryTurns[1].AnswerText != "Answer two" {
		t.Fatalf("openingTurnCtx.historyTurns[1].answerText = %q, want Answer two", openingTurnCtx.HistoryTurns[1].AnswerText)
	}
}

func TestBuildNextCriterionQuestion_NextAreaRetryExhaustedFallsBack(t *testing.T) {
	originalBackoffs := aiRetryBackoffs
	aiRetryBackoffs = nil
	t.Cleanup(func() {
		aiRetryBackoffs = originalBackoffs
	})

	ai := &qaAIClient{
		generateTurnFn: func(context.Context, *AITurnContext) (*AIResponse, error) {
			return nil, errors.New("provider unavailable")
		},
	}
	svc := newServiceForControlFlowTests(newQAServiceStore(), nil, ai)

	nextQuestion, err := svc.buildNextCriterionQuestion(context.Background(), "AP-7K9X-M2NF", &turnSnapshot{
		flowState:         &FlowState{QuestionNumber: 4},
		session:           activeSession("AP-7K9X-M2NF", "en"),
		preferredLanguage: "en",
		timeRemainingS:    1200,
	}, &criterionTurnPlan{
		decision: CriterionTurnDecision{Action: CriterionTurnActionNext},
		nextArea: "social_group",
		projectedAreas: []QuestionArea{
			{Area: "protected_ground", Status: AreaStatusComplete, QuestionsCount: 2},
			{Area: "social_group", Status: AreaStatusPending, QuestionsCount: 0},
		},
	}, &criterionTurnEvaluation{
		aiResult: &AIResponse{NextQuestion: "Unused next question"},
	})
	if err != nil {
		t.Fatalf("buildNextCriterionQuestion() error = %v", err)
	}
	if nextQuestion.question.TextEn != "Fallback social group question" {
		t.Fatalf("question.textEn = %q, want fallback question", nextQuestion.question.TextEn)
	}
	if !nextQuestion.substituted {
		t.Fatalf("substituted = %v, want true", nextQuestion.substituted)
	}
}

func TestBuildNextCriterionQuestion_AbortedNextAreaOpeningPropagatesError(t *testing.T) {
	originalBackoffs := aiRetryBackoffs
	aiRetryBackoffs = []time.Duration{time.Minute}
	t.Cleanup(func() {
		aiRetryBackoffs = originalBackoffs
	})

	ctx, cancel := context.WithCancel(context.Background())
	ai := &qaAIClient{
		generateTurnFn: func(callCtx context.Context, _ *AITurnContext) (*AIResponse, error) {
			cancel()
			return nil, errors.New("provider unavailable")
		},
	}
	svc := newServiceForControlFlowTests(newQAServiceStore(), nil, ai)

	_, err := svc.buildNextCriterionQuestion(ctx, "AP-7K9X-M2NF", &turnSnapshot{
		flowState:         &FlowState{QuestionNumber: 4},
		session:           activeSession("AP-7K9X-M2NF", "en"),
		preferredLanguage: "en",
		timeRemainingS:    1200,
	}, &criterionTurnPlan{
		decision: CriterionTurnDecision{Action: CriterionTurnActionNext},
		nextArea: "social_group",
		projectedAreas: []QuestionArea{
			{Area: "protected_ground", Status: AreaStatusComplete, QuestionsCount: 2},
			{Area: "social_group", Status: AreaStatusPending, QuestionsCount: 0},
		},
	}, &criterionTurnEvaluation{
		aiResult: &AIResponse{NextQuestion: "Unused next question"},
	})
	if err == nil {
		t.Fatalf("buildNextCriterionQuestion() error = nil, want non-nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context canceled", err)
	}
}
