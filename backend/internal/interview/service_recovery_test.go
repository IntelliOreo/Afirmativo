package interview

import (
	"context"
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

	store.prepareReadinessStepFn = func(_ context.Context, _ string, turnID string) (*FlowState, error) {
		return &FlowState{Step: FlowStepReadiness, ExpectedTurnID: turnID, QuestionNumber: 7}, nil
	}

	sessions := &fakeInterviewSessionStore{
		getSessionByCodeFn: func(context.Context, string) (*session.Session, error) {
			return activeSession(sessionCode, "en"), nil
		},
		startSessionFn: func(_ context.Context, _, _ string) (*session.Session, error) {
			return activeSession(sessionCode, "en"), nil
		},
	}

	svc := newServiceForRecoveryTests(store, sessions, &qaAIClient{})

	result, err := svc.StartInterview(context.Background(), sessionCode, "en")
	if err != nil {
		t.Fatalf("StartInterview() error = %v", err)
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

	ai := &qaAIClient{
		callFn: func(_ context.Context, _ *AITurnContext) (*AIResponse, error) {
			return &AIResponse{NextQuestion: "Please tell me about your first entry."}, nil
		},
	}

	svc := newServiceForRecoveryTests(store, sessions, ai)

	result, err := svc.SubmitAnswer(context.Background(), sessionCode, "Yes", "Ready", "turn-readiness")
	if err != nil {
		t.Fatalf("SubmitAnswer() error = %v", err)
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

func TestGetAnswerJobResult_CanceledJobExposesReloadRecoveryCode(t *testing.T) {
	t.Parallel()

	const (
		sessionCode = "AP-7K9X-M2NF"
		jobID       = "job-1"
		errorMsg    = "AI processing was unstable after retries. Reload to continue."
	)

	store := &fakeInterviewStore{
		getAnswerJobFn: func(_ context.Context, gotSessionCode, gotJobID string) (*AnswerJob, error) {
			return &AnswerJob{
				ID:              gotJobID,
				SessionCode:     gotSessionCode,
				ClientRequestID: "req-1",
				Status:          AsyncAnswerJobCanceled,
				ErrorCode:       "AI_RETRY_EXHAUSTED",
				ErrorMessage:    errorMsg,
			}, nil
		},
	}
	svc := newInterviewServiceForAsyncTests(store)

	got, err := svc.GetAnswerJobResult(context.Background(), sessionCode, jobID)
	if err != nil {
		t.Fatalf("GetAnswerJobResult() error = %v", err)
	}
	if got.Status != AsyncAnswerJobCanceled {
		t.Fatalf("status = %q, want %q", got.Status, AsyncAnswerJobCanceled)
	}
	if got.ErrorCode != "AI_RETRY_EXHAUSTED" {
		t.Fatalf("errorCode = %q, want AI_RETRY_EXHAUSTED", got.ErrorCode)
	}
	if got.ErrorMessage != errorMsg {
		t.Fatalf("errorMessage = %q, want %q", got.ErrorMessage, errorMsg)
	}
}

func TestSubmitAnswer_CriterionStepCompletesInterviewAcrossSessionLanguages(t *testing.T) {
	tests := []struct {
		name            string
		sessionLanguage string
	}{
		{
			name:            "english_session",
			sessionLanguage: "en",
		},
		{
			name:            "spanish_session",
			sessionLanguage: "es",
		},
		{
			name:            "unknown_language_defaults_internally",
			sessionLanguage: "fr",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sessionCode := "AP-7K9X-M2NF"
			const answerText = "Candidate answer from frontend"
			const questionText = "Criterion question text"
			const turnID = "turn-criterion"

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
				}, nil
			}
			store.getInProgressAreaFn = func(context.Context, string) (*QuestionArea, error) {
				return &QuestionArea{Area: "protected_ground", Status: AreaStatusInProgress, QuestionsCount: 1}, nil
			}
			store.getAnswersBySessionFn = func(context.Context, string) ([]Answer, error) {
				return []Answer{}, nil
			}

			store.processCriterionTurnFn = func(_ context.Context, _ ProcessCriterionTurnParams) (*ProcessCriterionTurnResult, error) {
				return &ProcessCriterionTurnResult{
					Action:         "next",
					NextArea:       "",
					QuestionNumber: 5,
				}, nil
			}

			sessions := &fakeInterviewSessionStore{
				getSessionByCodeFn: func(context.Context, string) (*session.Session, error) {
					return activeSession(sessionCode, tc.sessionLanguage), nil
				},
			}

			ai := &qaAIClient{
				callFn: func(context.Context, *AITurnContext) (*AIResponse, error) {
					return &AIResponse{
						Evaluation: &Evaluation{
							CurrentCriterion: CurrentCriterion{
								ID:              1,
								Status:          "sufficient",
								EvidenceSummary: "English evidence summary",
								Recommendation:  "move_on",
							},
							OtherCriteriaAddressed: nil,
						},
						NextQuestion: "Any fallback next question",
					}, nil
				},
			}

			svc := newServiceForRecoveryTests(store, sessions, ai)

			result, err := svc.SubmitAnswer(context.Background(), sessionCode, answerText, questionText, turnID)
			if err != nil {
				t.Fatalf("SubmitAnswer() error = %v", err)
			}
			if !result.Done {
				t.Fatalf("done = %v, want true when ProcessCriterionTurn returns no next area", result.Done)
			}
			if result.NextQuestion != nil {
				t.Fatalf("nextQuestion = %#v, want nil when interview is complete", result.NextQuestion)
			}
		})
	}
}
