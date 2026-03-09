package interview

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/afirmativo/backend/internal/config"
	"github.com/afirmativo/backend/internal/session"
)

func newServiceForControlFlowTests(store Store, sessions *fakeInterviewSessionStore, ai InterviewAIClient) *Service {
	if sessions == nil {
		sessions = &fakeInterviewSessionStore{}
	}
	return NewService(
		sessions,
		sessions,
		sessions,
		store,
		ai,
		[]config.AreaConfig{
			{
				ID:                      1,
				Slug:                    "protected_ground",
				Label:                   "Protected ground",
				Description:             "Protected ground description",
				SufficiencyRequirements: "Protected ground sufficiency requirements",
				FallbackQuestion:        "Fallback protected ground question",
			},
			{
				ID:                      2,
				Slug:                    "social_group",
				Label:                   "Social group",
				Description:             "Social group description",
				SufficiencyRequirements: "Social group sufficiency requirements",
				FallbackQuestion:        "Fallback social group question",
			},
		},
		"Opening disclaimer EN",
		"Opening disclaimer ES",
		"Default readiness EN",
		"Default readiness ES",
		AsyncConfig{},
	)
}

func TestProcessTurn_TimeoutFinishesInterview(t *testing.T) {
	const (
		sessionCode = "AP-7K9X-M2NF"
		turnID      = "turn-timeout"
	)

	store := newQAServiceStore()
	store.getFlowStateFn = func(context.Context, string) (*FlowState, error) {
		return &FlowState{
			Step:           FlowStepCriterion,
			ExpectedTurnID: turnID,
			QuestionNumber: 4,
		}, nil
	}
	store.getAreasBySessionFn = func(context.Context, string) ([]QuestionArea, error) {
		return []QuestionArea{
			{Area: "protected_ground", Status: AreaStatusInProgress},
			{Area: "social_group", Status: AreaStatusPending},
			{Area: "political_opinion", Status: AreaStatusComplete},
		}, nil
	}
	store.getInProgressAreaFn = func(context.Context, string) (*QuestionArea, error) {
		return &QuestionArea{Area: "protected_ground", Status: AreaStatusInProgress}, nil
	}

	markedNotAssessed := make([]string, 0, 2)
	store.fakeInterviewStore.markAreaNotAssessedFn = func(_ context.Context, gotSessionCode, area string) error {
		if gotSessionCode != sessionCode {
			t.Fatalf("MarkAreaNotAssessed() sessionCode = %q, want %q", gotSessionCode, sessionCode)
		}
		markedNotAssessed = append(markedNotAssessed, area)
		return nil
	}

	markFlowDoneCalls := 0
	store.fakeInterviewStore.markFlowDoneFn = func(_ context.Context, gotSessionCode string) error {
		if gotSessionCode != sessionCode {
			t.Fatalf("MarkFlowDone() sessionCode = %q, want %q", gotSessionCode, sessionCode)
		}
		markFlowDoneCalls++
		return nil
	}

	completeSessionCalls := 0
	sessions := &fakeInterviewSessionStore{
		getSessionByCodeFn: func(context.Context, string) (*session.Session, error) {
			return &session.Session{
				SessionCode:            sessionCode,
				PreferredLanguage:      "en",
				Status:                 "interviewing",
				InterviewBudgetSeconds: 600,
				InterviewLapsedSeconds: 600,
				ExpiresAt:              time.Now().UTC().Add(24 * time.Hour),
			}, nil
		},
		completeSessionFn: func(_ context.Context, gotSessionCode string) error {
			if gotSessionCode != sessionCode {
				t.Fatalf("CompleteSession() sessionCode = %q, want %q", gotSessionCode, sessionCode)
			}
			completeSessionCalls++
			return nil
		},
	}

	svc := newServiceForRecoveryTests(store, sessions, &qaAIClient{})

	got, err := svc.processTurn(context.Background(), sessionCode, "Answer", "Question", turnID)
	if err != nil {
		t.Fatalf("processTurn() error = %v", err)
	}
	if !got.Done {
		t.Fatalf("done = %v, want true", got.Done)
	}
	if got.NextQuestion != nil {
		t.Fatalf("nextQuestion = %#v, want nil", got.NextQuestion)
	}
	if got.TimerRemainingS != 0 {
		t.Fatalf("timerRemainingS = %d, want 0", got.TimerRemainingS)
	}
	if markFlowDoneCalls != 1 {
		t.Fatalf("MarkFlowDone() calls = %d, want 1", markFlowDoneCalls)
	}
	if completeSessionCalls != 1 {
		t.Fatalf("CompleteSession() calls = %d, want 1", completeSessionCalls)
	}
	if len(markedNotAssessed) != 2 {
		t.Fatalf("marked not assessed = %v, want 2 unresolved areas", markedNotAssessed)
	}
	if markedNotAssessed[0] != "protected_ground" || markedNotAssessed[1] != "social_group" {
		t.Fatalf("marked not assessed = %v, want [protected_ground social_group]", markedNotAssessed)
	}
}

func TestProcessTurn_TimeoutStillFinishesWhenCleanupWritesFail(t *testing.T) {
	const (
		sessionCode = "AP-7K9X-M2NF"
		turnID      = "turn-timeout-cleanup-failure"
	)

	store := newQAServiceStore()
	store.getFlowStateFn = func(context.Context, string) (*FlowState, error) {
		return &FlowState{
			Step:           FlowStepCriterion,
			ExpectedTurnID: turnID,
			QuestionNumber: 7,
		}, nil
	}
	store.getAreasBySessionFn = func(context.Context, string) ([]QuestionArea, error) {
		return []QuestionArea{
			{Area: "protected_ground", Status: AreaStatusPending},
			{Area: "social_group", Status: AreaStatusInProgress},
			{Area: "complete_area", Status: AreaStatusComplete},
			{Area: "insufficient_area", Status: AreaStatusInsufficient},
			{Area: "not_assessed_area", Status: AreaStatusNotAssessed},
			{Area: "pre_addressed_area", Status: AreaStatusPreAddressed},
		}, nil
	}
	store.getInProgressAreaFn = func(context.Context, string) (*QuestionArea, error) {
		return &QuestionArea{Area: "social_group", Status: AreaStatusInProgress}, nil
	}

	markedNotAssessed := make([]string, 0, 3)
	store.fakeInterviewStore.markAreaNotAssessedFn = func(_ context.Context, gotSessionCode, area string) error {
		if gotSessionCode != sessionCode {
			t.Fatalf("MarkAreaNotAssessed() sessionCode = %q, want %q", gotSessionCode, sessionCode)
		}
		markedNotAssessed = append(markedNotAssessed, area)
		return fmt.Errorf("write failed for %s", area)
	}

	markFlowDoneCalls := 0
	store.fakeInterviewStore.markFlowDoneFn = func(_ context.Context, gotSessionCode string) error {
		if gotSessionCode != sessionCode {
			t.Fatalf("MarkFlowDone() sessionCode = %q, want %q", gotSessionCode, sessionCode)
		}
		markFlowDoneCalls++
		return errors.New("mark flow done failed")
	}

	completeSessionCalls := 0
	sessions := &fakeInterviewSessionStore{
		getSessionByCodeFn: func(context.Context, string) (*session.Session, error) {
			return &session.Session{
				SessionCode:            sessionCode,
				PreferredLanguage:      "en",
				Status:                 "interviewing",
				InterviewBudgetSeconds: 600,
				InterviewLapsedSeconds: 600,
				ExpiresAt:              time.Now().UTC().Add(24 * time.Hour),
			}, nil
		},
		completeSessionFn: func(_ context.Context, gotSessionCode string) error {
			if gotSessionCode != sessionCode {
				t.Fatalf("CompleteSession() sessionCode = %q, want %q", gotSessionCode, sessionCode)
			}
			completeSessionCalls++
			return nil
		},
	}

	svc := newServiceForRecoveryTests(store, sessions, &qaAIClient{})

	got, err := svc.processTurn(context.Background(), sessionCode, "Answer", "Question", turnID)
	if err != nil {
		t.Fatalf("processTurn() error = %v", err)
	}
	if !got.Done {
		t.Fatalf("done = %v, want true", got.Done)
	}
	if got.TimerRemainingS != 0 {
		t.Fatalf("timerRemainingS = %d, want 0", got.TimerRemainingS)
	}
	if markFlowDoneCalls != 1 {
		t.Fatalf("MarkFlowDone() calls = %d, want 1", markFlowDoneCalls)
	}
	if completeSessionCalls != 1 {
		t.Fatalf("CompleteSession() calls = %d, want 1", completeSessionCalls)
	}
	wantMarked := []string{"protected_ground", "social_group", "pre_addressed_area"}
	if len(markedNotAssessed) != len(wantMarked) {
		t.Fatalf("marked not assessed = %v, want %v", markedNotAssessed, wantMarked)
	}
	for i, want := range wantMarked {
		if markedNotAssessed[i] != want {
			t.Fatalf("marked not assessed[%d] = %q, want %q", i, markedNotAssessed[i], want)
		}
	}
}

func TestProcessTurn_NoCurrentAreaFinishesByStep(t *testing.T) {
	tests := []struct {
		name             string
		step             FlowStep
		wantMarkFlowDone int
	}{
		{
			name:             "disclaimer_step_finishes_without_marking_flow_done",
			step:             FlowStepDisclaimer,
			wantMarkFlowDone: 0,
		},
		{
			name:             "readiness_step_finishes_without_marking_flow_done",
			step:             FlowStepReadiness,
			wantMarkFlowDone: 0,
		},
		{
			name:             "criterion_step_finishes_and_marks_flow_done",
			step:             FlowStepCriterion,
			wantMarkFlowDone: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			const sessionCode = "AP-7K9X-M2NF"
			const turnID = "turn-no-current-area"

			store := newQAServiceStore()
			store.getFlowStateFn = func(context.Context, string) (*FlowState, error) {
				return &FlowState{
					Step:           tc.step,
					ExpectedTurnID: turnID,
					QuestionNumber: 3,
				}, nil
			}
			store.getAreasBySessionFn = func(context.Context, string) ([]QuestionArea, error) {
				return []QuestionArea{
					{Area: "protected_ground", Status: AreaStatusComplete},
				}, nil
			}
			store.getInProgressAreaFn = func(context.Context, string) (*QuestionArea, error) {
				return nil, nil
			}

			markFlowDoneCalls := 0
			store.fakeInterviewStore.markFlowDoneFn = func(_ context.Context, gotSessionCode string) error {
				if gotSessionCode != sessionCode {
					t.Fatalf("MarkFlowDone() sessionCode = %q, want %q", gotSessionCode, sessionCode)
				}
				markFlowDoneCalls++
				return nil
			}

			completeSessionCalls := 0
			sessions := &fakeInterviewSessionStore{
				getSessionByCodeFn: func(context.Context, string) (*session.Session, error) {
					return activeSession(sessionCode, "en"), nil
				},
				completeSessionFn: func(_ context.Context, gotSessionCode string) error {
					if gotSessionCode != sessionCode {
						t.Fatalf("CompleteSession() sessionCode = %q, want %q", gotSessionCode, sessionCode)
					}
					completeSessionCalls++
					return nil
				},
			}

			svc := newServiceForControlFlowTests(store, sessions, &qaAIClient{})

			got, err := svc.processTurn(context.Background(), sessionCode, "Answer", "Question", turnID)
			if err != nil {
				t.Fatalf("processTurn() error = %v", err)
			}
			if !got.Done {
				t.Fatalf("done = %v, want true", got.Done)
			}
			if got.TimerRemainingS != 0 {
				t.Fatalf("timerRemainingS = %d, want 0", got.TimerRemainingS)
			}
			if markFlowDoneCalls != tc.wantMarkFlowDone {
				t.Fatalf("MarkFlowDone() calls = %d, want %d", markFlowDoneCalls, tc.wantMarkFlowDone)
			}
			if completeSessionCalls != 1 {
				t.Fatalf("CompleteSession() calls = %d, want 1", completeSessionCalls)
			}
		})
	}
}

func TestProcessTurn_NextAreaUsesFallbackQuestionOnAIExhaustion(t *testing.T) {
	const (
		sessionCode = "AP-7K9X-M2NF"
		turnID      = "turn-next-ai-exhausted"
	)

	originalBackoffs := aiRetryBackoffs
	aiRetryBackoffs = nil
	t.Cleanup(func() {
		aiRetryBackoffs = originalBackoffs
	})

	store := newQAServiceStore()
	store.getFlowStateFn = func(context.Context, string) (*FlowState, error) {
		return &FlowState{
			Step:           FlowStepCriterion,
			ExpectedTurnID: turnID,
			QuestionNumber: 4,
		}, nil
	}

	areasBySessionCalls := 0
	store.getAreasBySessionFn = func(context.Context, string) ([]QuestionArea, error) {
		areasBySessionCalls++
		if areasBySessionCalls == 1 {
			return []QuestionArea{
				{Area: "protected_ground", Status: AreaStatusInProgress, QuestionsCount: 1},
				{Area: "social_group", Status: AreaStatusPending, QuestionsCount: 0},
			}, nil
		}
		return []QuestionArea{
			{Area: "protected_ground", Status: AreaStatusComplete, QuestionsCount: 2},
			{Area: "social_group", Status: AreaStatusInProgress, QuestionsCount: 0},
		}, nil
	}

	inProgressCalls := 0
	store.getInProgressAreaFn = func(context.Context, string) (*QuestionArea, error) {
		inProgressCalls++
		if inProgressCalls == 1 {
			return &QuestionArea{Area: "protected_ground", Status: AreaStatusInProgress, QuestionsCount: 1}, nil
		}
		return &QuestionArea{Area: "social_group", Status: AreaStatusInProgress, QuestionsCount: 0}, nil
	}

	answerLoadCalls := 0
	store.getAnswersBySessionFn = func(context.Context, string) ([]Answer, error) {
		answerLoadCalls++
		return []Answer{
			{
				Area:         "protected_ground",
				QuestionText: "First question",
				TranscriptEn: "First answer",
			},
		}, nil
	}
	store.processCriterionTurnFn = func(_ context.Context, _ ProcessCriterionTurnParams) (*ProcessCriterionTurnResult, error) {
		return &ProcessCriterionTurnResult{
			Action:         "next",
			NextArea:       "social_group",
			QuestionNumber: 5,
		}, nil
	}

	sessions := &fakeInterviewSessionStore{
		getSessionByCodeFn: func(context.Context, string) (*session.Session, error) {
			return activeSession(sessionCode, "en"), nil
		},
	}

	aiCalls := 0
	ai := &qaAIClient{
		generateTurnFn: func(context.Context, *AITurnContext) (*AIResponse, error) {
			aiCalls++
			if aiCalls == 1 {
				return &AIResponse{
					Evaluation: &Evaluation{
						CurrentCriterion: CurrentCriterion{
							ID:              1,
							Status:          "sufficient",
							EvidenceSummary: "Evidence summary",
							Recommendation:  "move_on",
						},
					},
					NextQuestion: "Unused next question",
				}, nil
			}
			return nil, errors.New("provider unavailable")
		},
	}

	svc := newServiceForControlFlowTests(store, sessions, ai)

	got, err := svc.processTurn(context.Background(), sessionCode, "Answer", "Question", turnID)
	if err != nil {
		t.Fatalf("processTurn() error = %v", err)
	}
	if got.Done {
		t.Fatalf("done = %v, want false", got.Done)
	}
	if !got.Substituted {
		t.Fatalf("substituted = %v, want true", got.Substituted)
	}
	if got.NextQuestion == nil {
		t.Fatalf("nextQuestion = nil, want non-nil")
	}
	if got.NextQuestion.TextEn != "Fallback social group question" {
		t.Fatalf("nextQuestion.textEn = %q, want fallback question", got.NextQuestion.TextEn)
	}
	if got.NextQuestion.Area != "social_group" {
		t.Fatalf("nextQuestion.area = %q, want social_group", got.NextQuestion.Area)
	}
	if answerLoadCalls != 2 {
		t.Fatalf("GetAnswersBySession() calls = %d, want 2", answerLoadCalls)
	}
	if aiCalls != 2 {
		t.Fatalf("GenerateTurn() calls = %d, want 2", aiCalls)
	}
}

func TestProcessTurn_NextAreaAnswerLoadFailureFallsBackWithoutError(t *testing.T) {
	const (
		sessionCode = "AP-7K9X-M2NF"
		turnID      = "turn-next-answer-load-failure"
	)

	store := newQAServiceStore()
	store.getFlowStateFn = func(context.Context, string) (*FlowState, error) {
		return &FlowState{
			Step:           FlowStepCriterion,
			ExpectedTurnID: turnID,
			QuestionNumber: 4,
		}, nil
	}

	areasBySessionCalls := 0
	store.getAreasBySessionFn = func(context.Context, string) ([]QuestionArea, error) {
		areasBySessionCalls++
		if areasBySessionCalls == 1 {
			return []QuestionArea{
				{Area: "protected_ground", Status: AreaStatusInProgress, QuestionsCount: 1},
				{Area: "social_group", Status: AreaStatusPending, QuestionsCount: 0},
			}, nil
		}
		return []QuestionArea{
			{Area: "protected_ground", Status: AreaStatusComplete, QuestionsCount: 2},
			{Area: "social_group", Status: AreaStatusInProgress, QuestionsCount: 0},
		}, nil
	}

	inProgressCalls := 0
	store.getInProgressAreaFn = func(context.Context, string) (*QuestionArea, error) {
		inProgressCalls++
		if inProgressCalls == 1 {
			return &QuestionArea{Area: "protected_ground", Status: AreaStatusInProgress, QuestionsCount: 1}, nil
		}
		return &QuestionArea{Area: "social_group", Status: AreaStatusInProgress, QuestionsCount: 0}, nil
	}

	answerLoadCalls := 0
	store.getAnswersBySessionFn = func(context.Context, string) ([]Answer, error) {
		answerLoadCalls++
		if answerLoadCalls == 1 {
			return []Answer{
				{
					Area:         "protected_ground",
					QuestionText: "First question",
					TranscriptEn: "First answer",
				},
			}, nil
		}
		return nil, errors.New("db unavailable")
	}
	store.processCriterionTurnFn = func(_ context.Context, _ ProcessCriterionTurnParams) (*ProcessCriterionTurnResult, error) {
		return &ProcessCriterionTurnResult{
			Action:         "next",
			NextArea:       "social_group",
			QuestionNumber: 5,
		}, nil
	}

	sessions := &fakeInterviewSessionStore{
		getSessionByCodeFn: func(context.Context, string) (*session.Session, error) {
			return activeSession(sessionCode, "en"), nil
		},
	}

	aiCalls := 0
	ai := &qaAIClient{
		generateTurnFn: func(context.Context, *AITurnContext) (*AIResponse, error) {
			aiCalls++
			return &AIResponse{
				Evaluation: &Evaluation{
					CurrentCriterion: CurrentCriterion{
						ID:              1,
						Status:          "sufficient",
						EvidenceSummary: "Evidence summary",
						Recommendation:  "move_on",
					},
				},
				NextQuestion: "Unused next question",
			}, nil
		},
	}

	svc := newServiceForControlFlowTests(store, sessions, ai)

	got, err := svc.processTurn(context.Background(), sessionCode, "Answer", "Question", turnID)
	if err != nil {
		t.Fatalf("processTurn() error = %v", err)
	}
	if got.Done {
		t.Fatalf("done = %v, want false", got.Done)
	}
	if got.Substituted {
		t.Fatalf("substituted = %v, want false", got.Substituted)
	}
	if got.NextQuestion == nil {
		t.Fatalf("nextQuestion = nil, want non-nil")
	}
	if got.NextQuestion.TextEn != "Fallback social group question" {
		t.Fatalf("nextQuestion.textEn = %q, want fallback question", got.NextQuestion.TextEn)
	}
	if got.NextQuestion.Area != "social_group" {
		t.Fatalf("nextQuestion.area = %q, want social_group", got.NextQuestion.Area)
	}
	if answerLoadCalls != 2 {
		t.Fatalf("GetAnswersBySession() calls = %d, want 2", answerLoadCalls)
	}
	if aiCalls != 1 {
		t.Fatalf("GenerateTurn() calls = %d, want 1", aiCalls)
	}
}

func TestProcessTurn_NextAreaUsesAIQuestionAndBuildsOpeningContext(t *testing.T) {
	const (
		sessionCode = "AP-7K9X-M2NF"
		turnID      = "turn-next-area-success"
	)

	store := newQAServiceStore()
	store.getFlowStateFn = func(context.Context, string) (*FlowState, error) {
		return &FlowState{
			Step:           FlowStepCriterion,
			ExpectedTurnID: turnID,
			QuestionNumber: 4,
		}, nil
	}

	areasBySessionCalls := 0
	store.getAreasBySessionFn = func(context.Context, string) ([]QuestionArea, error) {
		areasBySessionCalls++
		if areasBySessionCalls == 1 {
			return []QuestionArea{
				{Area: "protected_ground", Status: AreaStatusInProgress, QuestionsCount: 2},
				{Area: "social_group", Status: AreaStatusPreAddressed, QuestionsCount: 1},
			}, nil
		}
		return []QuestionArea{
			{Area: "protected_ground", Status: AreaStatusComplete, QuestionsCount: 3},
			{Area: "social_group", Status: AreaStatusPreAddressed, QuestionsCount: 1},
		}, nil
	}

	inProgressCalls := 0
	store.getInProgressAreaFn = func(context.Context, string) (*QuestionArea, error) {
		inProgressCalls++
		if inProgressCalls == 1 {
			return &QuestionArea{Area: "protected_ground", Status: AreaStatusInProgress, QuestionsCount: 2}, nil
		}
		return &QuestionArea{Area: "social_group", Status: AreaStatusPreAddressed, QuestionsCount: 1}, nil
	}

	answerLoadCalls := 0
	store.getAnswersBySessionFn = func(context.Context, string) ([]Answer, error) {
		answerLoadCalls++
		switch answerLoadCalls {
		case 1:
			return []Answer{
				{
					Area:         "protected_ground",
					QuestionText: "Initial question",
					TranscriptEn: "Current-area answer",
				},
			}, nil
		case 2:
			return []Answer{
				{
					Area:         "protected_ground",
					QuestionText: "Question one",
					TranscriptEn: "",
					TranscriptEs: "Respuesta uno",
				},
				{
					Area:         "protected_ground",
					QuestionText: "Question two",
					TranscriptEn: "Answer two",
					TranscriptEs: "Respuesta dos",
				},
			}, nil
		default:
			t.Fatalf("unexpected GetAnswersBySession() call %d", answerLoadCalls)
			return nil, nil
		}
	}

	store.processCriterionTurnFn = func(_ context.Context, _ ProcessCriterionTurnParams) (*ProcessCriterionTurnResult, error) {
		return &ProcessCriterionTurnResult{
			Action:         "next",
			NextArea:       "social_group",
			QuestionNumber: 5,
		}, nil
	}

	sessions := &fakeInterviewSessionStore{
		getSessionByCodeFn: func(context.Context, string) (*session.Session, error) {
			return activeSession(sessionCode, "en"), nil
		},
	}

	aiCalls := 0
	var criterionTurnCtx *AITurnContext
	var openingTurnCtx *AITurnContext
	ai := &qaAIClient{
		generateTurnFn: func(_ context.Context, turnCtx *AITurnContext) (*AIResponse, error) {
			aiCalls++
			if aiCalls == 1 {
				criterionTurnCtx = turnCtx
				if turnCtx.IsOpeningTurn {
					t.Fatalf("first AI call isOpeningTurn = %v, want false", turnCtx.IsOpeningTurn)
				}
				return &AIResponse{
					Evaluation: &Evaluation{
						CurrentCriterion: CurrentCriterion{
							ID:              1,
							Status:          "sufficient",
							EvidenceSummary: "Evidence summary",
							Recommendation:  "move_on",
						},
					},
					NextQuestion: "Unused next question",
				}, nil
			}
			openingTurnCtx = turnCtx
			return &AIResponse{
				NextQuestion: "Please explain your social group claim.",
			}, nil
		},
	}

	svc := newServiceForControlFlowTests(store, sessions, ai)

	got, err := svc.processTurn(context.Background(), sessionCode, "Answer", "Question", turnID)
	if err != nil {
		t.Fatalf("processTurn() error = %v", err)
	}
	if got.Done {
		t.Fatalf("done = %v, want false", got.Done)
	}
	if got.Substituted {
		t.Fatalf("substituted = %v, want false", got.Substituted)
	}
	if got.NextQuestion == nil {
		t.Fatalf("nextQuestion = nil, want non-nil")
	}
	if got.NextQuestion.TextEn != "Please explain your social group claim." {
		t.Fatalf("nextQuestion.textEn = %q, want AI-generated next-area question", got.NextQuestion.TextEn)
	}
	if got.NextQuestion.Area != "social_group" {
		t.Fatalf("nextQuestion.area = %q, want social_group", got.NextQuestion.Area)
	}
	if answerLoadCalls != 2 {
		t.Fatalf("GetAnswersBySession() calls = %d, want 2", answerLoadCalls)
	}
	if aiCalls != 2 {
		t.Fatalf("GenerateTurn() calls = %d, want 2", aiCalls)
	}
	if criterionTurnCtx == nil {
		t.Fatalf("criterionTurnCtx = nil, want captured context")
	}
	if criterionTurnCtx.CurrentQuestionText != "Question" {
		t.Fatalf("criterionTurnCtx.currentQuestionText = %q, want Question", criterionTurnCtx.CurrentQuestionText)
	}
	if criterionTurnCtx.LatestAnswerText != "Answer" {
		t.Fatalf("criterionTurnCtx.latestAnswerText = %q, want Answer", criterionTurnCtx.LatestAnswerText)
	}
	if openingTurnCtx == nil {
		t.Fatalf("openingTurnCtx = nil, want captured context")
	}
	if !openingTurnCtx.IsOpeningTurn {
		t.Fatalf("openingTurnCtx.isOpeningTurn = %v, want true", openingTurnCtx.IsOpeningTurn)
	}
	if openingTurnCtx.CurrentAreaSlug != "social_group" {
		t.Fatalf("openingTurnCtx.currentAreaSlug = %q, want social_group", openingTurnCtx.CurrentAreaSlug)
	}
	if openingTurnCtx.AreaStatus != AreaStatusPreAddressed {
		t.Fatalf("openingTurnCtx.areaStatus = %q, want %q", openingTurnCtx.AreaStatus, AreaStatusPreAddressed)
	}
	if !openingTurnCtx.IsPreAddressed {
		t.Fatalf("openingTurnCtx.isPreAddressed = %v, want true", openingTurnCtx.IsPreAddressed)
	}
	if openingTurnCtx.FollowUpsRemaining != MaxQuestionsPerArea-1 {
		t.Fatalf("openingTurnCtx.followUpsRemaining = %d, want %d", openingTurnCtx.FollowUpsRemaining, MaxQuestionsPerArea-1)
	}
	if openingTurnCtx.QuestionsRemaining != EstimatedTotalQuestions-2 {
		t.Fatalf("openingTurnCtx.questionsRemaining = %d, want %d", openingTurnCtx.QuestionsRemaining, EstimatedTotalQuestions-2)
	}
	if openingTurnCtx.CriteriaRemaining != 1 {
		t.Fatalf("openingTurnCtx.criteriaRemaining = %d, want 1", openingTurnCtx.CriteriaRemaining)
	}
	if len(openingTurnCtx.CriteriaCoverage) != 2 {
		t.Fatalf("openingTurnCtx.criteriaCoverage length = %d, want 2", len(openingTurnCtx.CriteriaCoverage))
	}
	if len(openingTurnCtx.HistoryTurns) != 2 {
		t.Fatalf("openingTurnCtx.historyTurns length = %d, want 2", len(openingTurnCtx.HistoryTurns))
	}
	if openingTurnCtx.HistoryTurns[0].AnswerText != "Respuesta uno" {
		t.Fatalf("openingTurnCtx.historyTurns[0].answerText = %q, want spanish fallback", openingTurnCtx.HistoryTurns[0].AnswerText)
	}
	if openingTurnCtx.HistoryTurns[1].AnswerText != "Answer two" {
		t.Fatalf("openingTurnCtx.historyTurns[1].answerText = %q, want english transcript", openingTurnCtx.HistoryTurns[1].AnswerText)
	}
}

func TestProcessTurn_NextAreaEmptyAIQuestionFallsBack(t *testing.T) {
	const (
		sessionCode = "AP-7K9X-M2NF"
		turnID      = "turn-next-empty-question"
	)

	store := newQAServiceStore()
	store.getFlowStateFn = func(context.Context, string) (*FlowState, error) {
		return &FlowState{
			Step:           FlowStepCriterion,
			ExpectedTurnID: turnID,
			QuestionNumber: 4,
		}, nil
	}
	store.getAreasBySessionFn = func(context.Context, string) ([]QuestionArea, error) {
		return []QuestionArea{
			{Area: "protected_ground", Status: AreaStatusInProgress, QuestionsCount: 1},
			{Area: "social_group", Status: AreaStatusPending, QuestionsCount: 0},
		}, nil
	}
	store.getInProgressAreaFn = func(context.Context, string) (*QuestionArea, error) {
		return &QuestionArea{Area: "protected_ground", Status: AreaStatusInProgress, QuestionsCount: 1}, nil
	}

	answerLoadCalls := 0
	store.getAnswersBySessionFn = func(context.Context, string) ([]Answer, error) {
		answerLoadCalls++
		return []Answer{
			{
				Area:         "protected_ground",
				QuestionText: "First question",
				TranscriptEn: "First answer",
			},
		}, nil
	}
	store.processCriterionTurnFn = func(_ context.Context, _ ProcessCriterionTurnParams) (*ProcessCriterionTurnResult, error) {
		return &ProcessCriterionTurnResult{
			Action:         "next",
			NextArea:       "social_group",
			QuestionNumber: 5,
		}, nil
	}

	sessions := &fakeInterviewSessionStore{
		getSessionByCodeFn: func(context.Context, string) (*session.Session, error) {
			return activeSession(sessionCode, "en"), nil
		},
	}

	aiCalls := 0
	ai := &qaAIClient{
		generateTurnFn: func(context.Context, *AITurnContext) (*AIResponse, error) {
			aiCalls++
			if aiCalls == 1 {
				return &AIResponse{
					Evaluation: &Evaluation{
						CurrentCriterion: CurrentCriterion{
							ID:              1,
							Status:          "sufficient",
							EvidenceSummary: "Evidence summary",
							Recommendation:  "move_on",
						},
					},
					NextQuestion: "Unused next question",
				}, nil
			}
			return &AIResponse{NextQuestion: "   "}, nil
		},
	}

	svc := newServiceForControlFlowTests(store, sessions, ai)

	got, err := svc.processTurn(context.Background(), sessionCode, "Answer", "Question", turnID)
	if err != nil {
		t.Fatalf("processTurn() error = %v", err)
	}
	if got.Done {
		t.Fatalf("done = %v, want false", got.Done)
	}
	if !got.Substituted {
		t.Fatalf("substituted = %v, want true", got.Substituted)
	}
	if got.NextQuestion == nil {
		t.Fatalf("nextQuestion = nil, want non-nil")
	}
	if got.NextQuestion.TextEn != "Fallback social group question" {
		t.Fatalf("nextQuestion.textEn = %q, want fallback question", got.NextQuestion.TextEn)
	}
	if answerLoadCalls != 2 {
		t.Fatalf("GetAnswersBySession() calls = %d, want 2", answerLoadCalls)
	}
}

func TestProcessTurn_NextAreaProviderAbortPropagatesError(t *testing.T) {
	const (
		sessionCode = "AP-7K9X-M2NF"
		turnID      = "turn-next-provider-abort"
	)

	originalBackoffs := aiRetryBackoffs
	aiRetryBackoffs = []time.Duration{time.Minute}
	t.Cleanup(func() {
		aiRetryBackoffs = originalBackoffs
	})

	store := newQAServiceStore()
	store.getFlowStateFn = func(context.Context, string) (*FlowState, error) {
		return &FlowState{
			Step:           FlowStepCriterion,
			ExpectedTurnID: turnID,
			QuestionNumber: 4,
		}, nil
	}
	store.getAreasBySessionFn = func(context.Context, string) ([]QuestionArea, error) {
		return []QuestionArea{
			{Area: "protected_ground", Status: AreaStatusInProgress, QuestionsCount: 1},
			{Area: "social_group", Status: AreaStatusPending, QuestionsCount: 0},
		}, nil
	}
	store.getInProgressAreaFn = func(context.Context, string) (*QuestionArea, error) {
		return &QuestionArea{Area: "protected_ground", Status: AreaStatusInProgress, QuestionsCount: 1}, nil
	}
	store.getAnswersBySessionFn = func(context.Context, string) ([]Answer, error) {
		return []Answer{
			{
				Area:         "protected_ground",
				QuestionText: "First question",
				TranscriptEn: "First answer",
			},
		}, nil
	}
	store.processCriterionTurnFn = func(_ context.Context, _ ProcessCriterionTurnParams) (*ProcessCriterionTurnResult, error) {
		return &ProcessCriterionTurnResult{
			Action:         "next",
			NextArea:       "social_group",
			QuestionNumber: 5,
		}, nil
	}

	sessions := &fakeInterviewSessionStore{
		getSessionByCodeFn: func(context.Context, string) (*session.Session, error) {
			return activeSession(sessionCode, "en"), nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	aiCalls := 0
	ai := &qaAIClient{
		generateTurnFn: func(callCtx context.Context, _ *AITurnContext) (*AIResponse, error) {
			aiCalls++
			if aiCalls == 1 {
				return &AIResponse{
					Evaluation: &Evaluation{
						CurrentCriterion: CurrentCriterion{
							ID:              1,
							Status:          "sufficient",
							EvidenceSummary: "Evidence summary",
							Recommendation:  "move_on",
						},
					},
					NextQuestion: "Unused next question",
				}, nil
			}
			cancel()
			return nil, errors.New("provider temporarily unavailable")
		},
	}

	svc := newServiceForControlFlowTests(store, sessions, ai)

	_, err := svc.processTurn(ctx, sessionCode, "Answer", "Question", turnID)
	if err == nil {
		t.Fatalf("processTurn() error = nil, want propagated retry abort")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("processTurn() error = %v, want context canceled", err)
	}
}

func TestProcessTurn_NextAreaMissingFromStateSkipsOpeningAI(t *testing.T) {
	const (
		sessionCode = "AP-7K9X-M2NF"
		turnID      = "turn-next-area-missing"
	)

	store := newQAServiceStore()
	store.getFlowStateFn = func(context.Context, string) (*FlowState, error) {
		return &FlowState{
			Step:           FlowStepCriterion,
			ExpectedTurnID: turnID,
			QuestionNumber: 4,
		}, nil
	}

	areasBySessionCalls := 0
	store.getAreasBySessionFn = func(context.Context, string) ([]QuestionArea, error) {
		areasBySessionCalls++
		if areasBySessionCalls == 1 {
			return []QuestionArea{
				{Area: "protected_ground", Status: AreaStatusInProgress, QuestionsCount: 1},
				{Area: "social_group", Status: AreaStatusPending, QuestionsCount: 0},
			}, nil
		}
		return []QuestionArea{
			{Area: "protected_ground", Status: AreaStatusComplete, QuestionsCount: 2},
		}, nil
	}
	store.getInProgressAreaFn = func(context.Context, string) (*QuestionArea, error) {
		return &QuestionArea{Area: "protected_ground", Status: AreaStatusInProgress, QuestionsCount: 1}, nil
	}

	answerLoadCalls := 0
	store.getAnswersBySessionFn = func(context.Context, string) ([]Answer, error) {
		answerLoadCalls++
		return []Answer{
			{
				Area:         "protected_ground",
				QuestionText: "First question",
				TranscriptEn: "First answer",
			},
		}, nil
	}
	store.processCriterionTurnFn = func(_ context.Context, _ ProcessCriterionTurnParams) (*ProcessCriterionTurnResult, error) {
		return &ProcessCriterionTurnResult{
			Action:         "next",
			NextArea:       "social_group",
			QuestionNumber: 5,
		}, nil
	}

	sessions := &fakeInterviewSessionStore{
		getSessionByCodeFn: func(context.Context, string) (*session.Session, error) {
			return activeSession(sessionCode, "en"), nil
		},
	}

	aiCalls := 0
	ai := &qaAIClient{
		generateTurnFn: func(context.Context, *AITurnContext) (*AIResponse, error) {
			aiCalls++
			return &AIResponse{
				Evaluation: &Evaluation{
					CurrentCriterion: CurrentCriterion{
						ID:              1,
						Status:          "sufficient",
						EvidenceSummary: "Evidence summary",
						Recommendation:  "move_on",
					},
				},
				NextQuestion: "Unused next question",
			}, nil
		},
	}

	svc := newServiceForControlFlowTests(store, sessions, ai)

	got, err := svc.processTurn(context.Background(), sessionCode, "Answer", "Question", turnID)
	if err != nil {
		t.Fatalf("processTurn() error = %v", err)
	}
	if got.Done {
		t.Fatalf("done = %v, want false", got.Done)
	}
	if got.Substituted {
		t.Fatalf("substituted = %v, want false", got.Substituted)
	}
	if got.NextQuestion == nil {
		t.Fatalf("nextQuestion = nil, want non-nil")
	}
	if got.NextQuestion.TextEn != "Fallback social group question" {
		t.Fatalf("nextQuestion.textEn = %q, want fallback question", got.NextQuestion.TextEn)
	}
	if answerLoadCalls != 1 {
		t.Fatalf("GetAnswersBySession() calls = %d, want 1", answerLoadCalls)
	}
	if aiCalls != 1 {
		t.Fatalf("GenerateTurn() calls = %d, want 1", aiCalls)
	}
}

func TestSelectTranscript(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		preferEnglish bool
		en            string
		es            string
		want          string
	}{
		{
			name:          "english_preferred_uses_english",
			preferEnglish: true,
			en:            "Answer in English",
			es:            "Respuesta en espanol",
			want:          "Answer in English",
		},
		{
			name:          "english_preferred_falls_back_to_spanish",
			preferEnglish: true,
			en:            "   ",
			es:            "Respuesta en espanol",
			want:          "Respuesta en espanol",
		},
		{
			name:          "spanish_preferred_uses_spanish",
			preferEnglish: false,
			en:            "Answer in English",
			es:            "Respuesta en espanol",
			want:          "Respuesta en espanol",
		},
		{
			name:          "spanish_preferred_falls_back_to_english",
			preferEnglish: false,
			en:            "Answer in English",
			es:            " ",
			want:          "Answer in English",
		},
		{
			name:          "both_empty_returns_other_field_without_trimming",
			preferEnglish: true,
			en:            "",
			es:            "",
			want:          "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := selectTranscript(tc.preferEnglish, tc.en, tc.es); got != tc.want {
				t.Fatalf("selectTranscript(%v, %q, %q) = %q, want %q", tc.preferEnglish, tc.en, tc.es, got, tc.want)
			}
		})
	}
}
