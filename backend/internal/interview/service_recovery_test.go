package interview

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/afirmativo/backend/internal/config"
	"github.com/afirmativo/backend/internal/session"
)

type qaServiceStore struct {
	*fakeInterviewStore

	createQuestionAreaFn      func(ctx context.Context, sessionCode, area string) (*QuestionArea, error)
	setAreaInProgressFn       func(ctx context.Context, sessionCode, area string) error
	getInProgressAreaFn       func(ctx context.Context, sessionCode string) (*QuestionArea, error)
	getAreasBySessionFn       func(ctx context.Context, sessionCode string) ([]QuestionArea, error)
	getAnswersBySessionFn     func(ctx context.Context, sessionCode string) ([]Answer, error)
	getAnswerCountFn          func(ctx context.Context, sessionCode string) (int, error)
	getFlowStateFn            func(ctx context.Context, sessionCode string) (*FlowState, error)
	prepareDisclaimerStepFn   func(ctx context.Context, sessionCode, turnID string) (*FlowState, error)
	prepareReadinessStepFn    func(ctx context.Context, sessionCode, turnID string) (*FlowState, error)
	advanceNonCriterionStepFn func(ctx context.Context, params AdvanceNonCriterionStepParams) (*FlowState, error)
}

func newQAServiceStore() *qaServiceStore {
	return &qaServiceStore{fakeInterviewStore: &fakeInterviewStore{}}
}

func (s *qaServiceStore) CreateQuestionArea(ctx context.Context, sessionCode, area string) (*QuestionArea, error) {
	if s.createQuestionAreaFn != nil {
		return s.createQuestionAreaFn(ctx, sessionCode, area)
	}
	return s.fakeInterviewStore.CreateQuestionArea(ctx, sessionCode, area)
}

func (s *qaServiceStore) SetAreaInProgress(ctx context.Context, sessionCode, area string) error {
	if s.setAreaInProgressFn != nil {
		return s.setAreaInProgressFn(ctx, sessionCode, area)
	}
	return s.fakeInterviewStore.SetAreaInProgress(ctx, sessionCode, area)
}

func (s *qaServiceStore) GetInProgressArea(ctx context.Context, sessionCode string) (*QuestionArea, error) {
	if s.getInProgressAreaFn != nil {
		return s.getInProgressAreaFn(ctx, sessionCode)
	}
	return s.fakeInterviewStore.GetInProgressArea(ctx, sessionCode)
}

func (s *qaServiceStore) GetAreasBySession(ctx context.Context, sessionCode string) ([]QuestionArea, error) {
	if s.getAreasBySessionFn != nil {
		return s.getAreasBySessionFn(ctx, sessionCode)
	}
	return s.fakeInterviewStore.GetAreasBySession(ctx, sessionCode)
}

func (s *qaServiceStore) GetAnswersBySession(ctx context.Context, sessionCode string) ([]Answer, error) {
	if s.getAnswersBySessionFn != nil {
		return s.getAnswersBySessionFn(ctx, sessionCode)
	}
	return s.fakeInterviewStore.GetAnswersBySession(ctx, sessionCode)
}

func (s *qaServiceStore) GetAnswerCount(ctx context.Context, sessionCode string) (int, error) {
	if s.getAnswerCountFn != nil {
		return s.getAnswerCountFn(ctx, sessionCode)
	}
	return s.fakeInterviewStore.GetAnswerCount(ctx, sessionCode)
}

func (s *qaServiceStore) GetFlowState(ctx context.Context, sessionCode string) (*FlowState, error) {
	if s.getFlowStateFn != nil {
		return s.getFlowStateFn(ctx, sessionCode)
	}
	return s.fakeInterviewStore.GetFlowState(ctx, sessionCode)
}

func (s *qaServiceStore) PrepareDisclaimerStep(ctx context.Context, sessionCode, turnID string) (*FlowState, error) {
	if s.prepareDisclaimerStepFn != nil {
		return s.prepareDisclaimerStepFn(ctx, sessionCode, turnID)
	}
	return s.fakeInterviewStore.PrepareDisclaimerStep(ctx, sessionCode, turnID)
}

func (s *qaServiceStore) PrepareReadinessStep(ctx context.Context, sessionCode, turnID string) (*FlowState, error) {
	if s.prepareReadinessStepFn != nil {
		return s.prepareReadinessStepFn(ctx, sessionCode, turnID)
	}
	return s.fakeInterviewStore.PrepareReadinessStep(ctx, sessionCode, turnID)
}

func (s *qaServiceStore) AdvanceNonCriterionStep(ctx context.Context, params AdvanceNonCriterionStepParams) (*FlowState, error) {
	if s.advanceNonCriterionStepFn != nil {
		return s.advanceNonCriterionStepFn(ctx, params)
	}
	return s.fakeInterviewStore.AdvanceNonCriterionStep(ctx, params)
}

type qaAIClient struct {
	callFn func(ctx context.Context, turnCtx *AITurnContext) (*AIResponse, error)
}

func (f *qaAIClient) CallAI(ctx context.Context, turnCtx *AITurnContext) (*AIResponse, error) {
	if f.callFn != nil {
		return f.callFn(ctx, turnCtx)
	}
	return nil, nil
}

func newServiceForRecoveryTests(store Store, sessions *fakeInterviewSessionStore, ai AIClient) *Service {
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
				Description:             "Criterion description",
				SufficiencyRequirements: "Sufficiency requirements",
				FallbackQuestion:        "Fallback protected ground question",
			},
		},
		"Opening disclaimer EN",
		"Opening disclaimer ES",
		"Default readiness EN",
		"Default readiness ES",
	)
}

func activeSession(sessionCode, preferredLanguage string) *session.Session {
	now := time.Now().UTC()
	return &session.Session{
		SessionCode:               sessionCode,
		PreferredLanguage:         preferredLanguage,
		Status:                    "interviewing",
		InterviewBudgetSeconds:    3600,
		InterviewLapsedSeconds:    300,
		CurrentInterviewStartedAt: &now,
		ExpiresAt:                 now.Add(24 * time.Hour),
	}
}

func TestStartInterview_ResumingSessionReturnsReadinessReturningUserMessage(t *testing.T) {
	sessionCode := "AP-7K9X-M2NF"
	store := newQAServiceStore()
	store.getAnswerCountFn = func(context.Context, string) (int, error) {
		return 2, nil
	}
	store.getFlowStateFn = func(context.Context, string) (*FlowState, error) {
		return &FlowState{Step: FlowStepCriterion, QuestionNumber: 6}, nil
	}
	store.createQuestionAreaFn = func(_ context.Context, gotSessionCode, area string) (*QuestionArea, error) {
		return &QuestionArea{SessionCode: gotSessionCode, Area: area, Status: AreaStatusPending}, nil
	}
	store.setAreaInProgressFn = func(context.Context, string, string) error {
		return nil
	}
	store.getInProgressAreaFn = func(context.Context, string) (*QuestionArea, error) {
		return &QuestionArea{Area: "protected_ground", Status: AreaStatusInProgress}, nil
	}

	prepareReadinessCalled := false
	store.prepareReadinessStepFn = func(_ context.Context, _ string, turnID string) (*FlowState, error) {
		prepareReadinessCalled = true
		return &FlowState{Step: FlowStepReadiness, ExpectedTurnID: turnID, QuestionNumber: 7}, nil
	}
	store.prepareDisclaimerStepFn = func(context.Context, string, string) (*FlowState, error) {
		t.Fatalf("PrepareDisclaimerStep should not be used for resuming sessions")
		return nil, nil
	}

	sessions := &fakeInterviewSessionStore{
		getSessionByCodeFn: func(context.Context, string) (*session.Session, error) {
			return activeSession(sessionCode, "en"), nil
		},
		startSessionFn: func(_ context.Context, gotSessionCode, preferredLanguage string) (*session.Session, error) {
			if gotSessionCode != sessionCode {
				t.Fatalf("startSession sessionCode=%q, want %q", gotSessionCode, sessionCode)
			}
			if preferredLanguage != "en" {
				t.Fatalf("preferredLanguage=%q, want en", preferredLanguage)
			}
			return activeSession(sessionCode, "en"), nil
		},
	}

	svc := newServiceForRecoveryTests(store, sessions, &qaAIClient{})

	result, err := svc.StartInterview(context.Background(), sessionCode, "en")
	if err != nil {
		t.Fatalf("StartInterview() error = %v", err)
	}
	if !prepareReadinessCalled {
		t.Fatalf("expected PrepareReadinessStep to be called")
	}
	if !result.Resuming {
		t.Fatalf("resuming = %v, want true", result.Resuming)
	}
	if result.Question == nil {
		t.Fatalf("question = nil, want non-nil")
	}
	if result.Question.Kind != QuestionKindReadiness {
		t.Fatalf("question.kind = %q, want %q", result.Question.Kind, QuestionKindReadiness)
	}
	if result.Question.TextEn != ResumeQuestion("protected_ground").TextEn {
		t.Fatalf("question.textEn = %q, want returning-user message %q", result.Question.TextEn, ResumeQuestion("protected_ground").TextEn)
	}
	if result.Question.QuestionNumber != 7 {
		t.Fatalf("question.questionNumber = %d, want 7", result.Question.QuestionNumber)
	}
	if result.Language != "en" {
		t.Fatalf("language = %q, want en", result.Language)
	}
}

func TestSubmitAnswer_ReadinessStepTriggersNewAICallAndReturnsNewQuestion(t *testing.T) {
	sessionCode := "AP-7K9X-M2NF"
	store := newQAServiceStore()
	store.getFlowStateFn = func(context.Context, string) (*FlowState, error) {
		return &FlowState{Step: FlowStepReadiness, ExpectedTurnID: "turn-readiness", QuestionNumber: 2}, nil
	}
	store.getAreasBySessionFn = func(context.Context, string) ([]QuestionArea, error) {
		return []QuestionArea{{Area: "protected_ground", Status: AreaStatusInProgress, QuestionsCount: 0}}, nil
	}
	store.getInProgressAreaFn = func(context.Context, string) (*QuestionArea, error) {
		return &QuestionArea{Area: "protected_ground", Status: AreaStatusInProgress, QuestionsCount: 0}, nil
	}
	store.advanceNonCriterionStepFn = func(_ context.Context, params AdvanceNonCriterionStepParams) (*FlowState, error) {
		if params.CurrentStep != FlowStepReadiness || params.NextStep != FlowStepCriterion {
			t.Fatalf("advance step transition = %q -> %q, want readiness -> criterion", params.CurrentStep, params.NextStep)
		}
		if params.ExpectedTurnID != "turn-readiness" {
			t.Fatalf("expected turn id = %q, want turn-readiness", params.ExpectedTurnID)
		}
		return &FlowState{Step: FlowStepCriterion, ExpectedTurnID: params.NextTurnID, QuestionNumber: 3}, nil
	}
	store.getAnswersBySessionFn = func(context.Context, string) ([]Answer, error) {
		return []Answer{}, nil
	}

	sessions := &fakeInterviewSessionStore{
		getSessionByCodeFn: func(context.Context, string) (*session.Session, error) {
			return activeSession(sessionCode, "en"), nil
		},
	}

	aiCalls := 0
	var capturedTurnCtx *AITurnContext
	ai := &qaAIClient{
		callFn: func(_ context.Context, turnCtx *AITurnContext) (*AIResponse, error) {
			aiCalls++
			capturedCopy := *turnCtx
			capturedTurnCtx = &capturedCopy
			return &AIResponse{NextQuestion: "Please tell me about your first entry."}, nil
		},
	}

	svc := newServiceForRecoveryTests(store, sessions, ai)

	result, err := svc.SubmitAnswer(context.Background(), sessionCode, "Yes", "Ready", "turn-readiness")
	if err != nil {
		t.Fatalf("SubmitAnswer() error = %v", err)
	}
	if aiCalls != 1 {
		t.Fatalf("AI calls = %d, want 1", aiCalls)
	}
	if capturedTurnCtx == nil {
		t.Fatalf("expected AI turn context to be captured")
	}
	if !capturedTurnCtx.IsOpeningTurn {
		t.Fatalf("AI turn context IsOpeningTurn = %v, want true", capturedTurnCtx.IsOpeningTurn)
	}
	if capturedTurnCtx.CurrentAreaSlug != "protected_ground" {
		t.Fatalf("AI turn area = %q, want protected_ground", capturedTurnCtx.CurrentAreaSlug)
	}
	if result.Done {
		t.Fatalf("done = %v, want false", result.Done)
	}
	if result.Substituted {
		t.Fatalf("substituted = %v, want false", result.Substituted)
	}
	if result.NextQuestion == nil {
		t.Fatalf("nextQuestion = nil, want non-nil")
	}
	if result.NextQuestion.Kind != QuestionKindCriterion {
		t.Fatalf("nextQuestion.kind = %q, want %q", result.NextQuestion.Kind, QuestionKindCriterion)
	}
	if result.NextQuestion.TextEn != "Please tell me about your first entry." {
		t.Fatalf("nextQuestion.textEn = %q, want AI question", result.NextQuestion.TextEn)
	}
	if result.NextQuestion.QuestionNumber != 3 {
		t.Fatalf("nextQuestion.questionNumber = %d, want 3", result.NextQuestion.QuestionNumber)
	}
}

func TestProcessAnswerJob_AIRetryExhaustedMarksCanceledWithReloadRecoveryCode(t *testing.T) {
	sessionCode := "AP-7K9X-M2NF"
	jobID := "job-1"

	store := newQAServiceStore()
	store.claimQueuedAnswerJobFn = func(context.Context, string) (*AnswerJob, error) {
		return &AnswerJob{
			ID:              jobID,
			SessionCode:     sessionCode,
			ClientRequestID: "req-1",
			TurnID:          "turn-readiness",
			QuestionText:    "Are you ready?",
			AnswerText:      "Yes",
			Status:          AsyncAnswerJobRunning,
		}, nil
	}
	store.getFlowStateFn = func(context.Context, string) (*FlowState, error) {
		return &FlowState{Step: FlowStepReadiness, ExpectedTurnID: "turn-readiness", QuestionNumber: 2}, nil
	}
	store.getAreasBySessionFn = func(context.Context, string) ([]QuestionArea, error) {
		return []QuestionArea{{Area: "protected_ground", Status: AreaStatusInProgress, QuestionsCount: 0}}, nil
	}
	store.getInProgressAreaFn = func(context.Context, string) (*QuestionArea, error) {
		return &QuestionArea{Area: "protected_ground", Status: AreaStatusInProgress, QuestionsCount: 0}, nil
	}
	store.advanceNonCriterionStepFn = func(_ context.Context, params AdvanceNonCriterionStepParams) (*FlowState, error) {
		return &FlowState{Step: FlowStepCriterion, ExpectedTurnID: params.NextTurnID, QuestionNumber: 3}, nil
	}
	store.getAnswersBySessionFn = func(context.Context, string) ([]Answer, error) {
		return []Answer{}, nil
	}

	var markedFailed *MarkAnswerJobFailedParams
	store.markAnswerJobFailedFn = func(_ context.Context, params MarkAnswerJobFailedParams) error {
		copy := params
		markedFailed = &copy
		return nil
	}

	markSucceededCalled := false
	store.markAnswerJobOKFn = func(context.Context, string, []byte) error {
		markSucceededCalled = true
		return nil
	}

	sessions := &fakeInterviewSessionStore{
		getSessionByCodeFn: func(context.Context, string) (*session.Session, error) {
			return activeSession(sessionCode, "en"), nil
		},
	}

	aiCalls := 0
	ai := &qaAIClient{
		callFn: func(context.Context, *AITurnContext) (*AIResponse, error) {
			aiCalls++
			return nil, errors.New("provider unavailable")
		},
	}

	svc := newServiceForRecoveryTests(store, sessions, ai)

	svc.processAnswerJob(context.Background(), jobID)

	if aiCalls < 2 {
		t.Fatalf("AI calls = %d, want at least 2 (initial + retry)", aiCalls)
	}
	if markedFailed == nil {
		t.Fatalf("expected async answer job to be marked as failed/canceled")
	}
	if markedFailed.Status != AsyncAnswerJobCanceled {
		t.Fatalf("marked status = %q, want %q", markedFailed.Status, AsyncAnswerJobCanceled)
	}
	if markedFailed.ErrorCode != "AI_RETRY_EXHAUSTED" {
		t.Fatalf("errorCode = %q, want AI_RETRY_EXHAUSTED", markedFailed.ErrorCode)
	}
	if !strings.Contains(markedFailed.ErrorMessage, "Reload to continue") {
		t.Fatalf("errorMessage = %q, want reload guidance", markedFailed.ErrorMessage)
	}
	if markSucceededCalled {
		t.Fatalf("MarkAnswerJobSucceeded should not be called when retries are exhausted")
	}
}
