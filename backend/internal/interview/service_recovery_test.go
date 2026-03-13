package interview

import (
	"context"
	"errors"
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
	prepareDisclaimerStepFn   func(ctx context.Context, sessionCode string, issuedQuestion *IssuedQuestion) (*FlowState, error)
	prepareReadinessStepFn    func(ctx context.Context, sessionCode string, issuedQuestion *IssuedQuestion) (*FlowState, error)
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

func (s *qaServiceStore) PrepareDisclaimerStep(ctx context.Context, sessionCode string, issuedQuestion *IssuedQuestion) (*FlowState, error) {
	if s.prepareDisclaimerStepFn != nil {
		return s.prepareDisclaimerStepFn(ctx, sessionCode, issuedQuestion)
	}
	return s.fakeInterviewStore.PrepareDisclaimerStep(ctx, sessionCode, issuedQuestion)
}

func (s *qaServiceStore) PrepareReadinessStep(ctx context.Context, sessionCode string, issuedQuestion *IssuedQuestion) (*FlowState, error) {
	if s.prepareReadinessStepFn != nil {
		return s.prepareReadinessStepFn(ctx, sessionCode, issuedQuestion)
	}
	return s.fakeInterviewStore.PrepareReadinessStep(ctx, sessionCode, issuedQuestion)
}

func (s *qaServiceStore) AdvanceNonCriterionStep(ctx context.Context, params AdvanceNonCriterionStepParams) (*FlowState, error) {
	if s.advanceNonCriterionStepFn != nil {
		return s.advanceNonCriterionStepFn(ctx, params)
	}
	return s.fakeInterviewStore.AdvanceNonCriterionStep(ctx, params)
}

type qaAIClient struct {
	generateTurnFn func(ctx context.Context, turnCtx *AITurnContext) (*AIResponse, error)
}

func (f *qaAIClient) GenerateTurn(ctx context.Context, turnCtx *AITurnContext) (*AIResponse, error) {
	if f.generateTurnFn != nil {
		return f.generateTurnFn(ctx, turnCtx)
	}
	return nil, nil
}

func newServiceForRecoveryTests(store Store, sessions *fakeInterviewSessionStore, ai InterviewAIClient) *Service {
	if sessions == nil {
		sessions = &fakeInterviewSessionStore{}
	}
	settings := defaultInterviewSettings()
	settings.AreaConfigs = []config.AreaConfig{
		{
			ID:                      1,
			Slug:                    "protected_ground",
			Label:                   "Protected ground",
			Description:             "Criterion description",
			SufficiencyRequirements: "Sufficiency requirements",
			FallbackQuestion:        "Fallback protected ground question",
		},
	}
	return NewService(Deps{
		SessionStarter:   sessions,
		SessionGetter:    sessions,
		SessionCompleter: sessions,
		Store:            store,
		AIClient:         ai,
	}, settings)
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

func TestStartInterview_FirstEntryStillReturnsDisclaimer(t *testing.T) {
	sessionCode := "AP-7K9X-M2NF"
	store := newQAServiceStore()
	store.getAnswerCountFn = func(context.Context, string) (int, error) {
		return 0, nil
	}
	store.getFlowStateFn = func(context.Context, string) (*FlowState, error) {
		return &FlowState{Step: FlowStepDisclaimer, QuestionNumber: 1}, nil
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
	store.prepareDisclaimerStepFn = func(_ context.Context, _ string, issuedQuestion *IssuedQuestion) (*FlowState, error) {
		return &FlowState{
			Step:           FlowStepDisclaimer,
			ExpectedTurnID: issuedQuestion.Question.TurnID,
			QuestionNumber: 1,
			ActiveQuestion: issuedQuestion,
		}, nil
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
	if result.Resuming {
		t.Fatalf("resuming = %v, want false", result.Resuming)
	}
	if result.Question == nil {
		t.Fatalf("question = nil, want non-nil")
	}
	if result.Question.Kind != QuestionKindDisclaimer {
		t.Fatalf("question.kind = %q, want %q", result.Question.Kind, QuestionKindDisclaimer)
	}
	if result.Question.TextEn != "Opening disclaimer EN" {
		t.Fatalf("question.textEn = %q, want opening disclaimer", result.Question.TextEn)
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

	store.prepareReadinessStepFn = func(_ context.Context, _ string, issuedQuestion *IssuedQuestion) (*FlowState, error) {
		return &FlowState{
			Step:           FlowStepReadiness,
			ExpectedTurnID: issuedQuestion.Question.TurnID,
			QuestionNumber: 7,
			ActiveQuestion: issuedQuestion,
		}, nil
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
	if result.Question.QuestionNumber != 6 {
		t.Fatalf("question.questionNumber = %d, want 6", result.Question.QuestionNumber)
	}
	if result.Language != "en" {
		t.Fatalf("language = %q, want en", result.Language)
	}
}

func TestStartInterview_ResumeCriterionAfterLongOfflineGapUsesCappedLiveElapsed(t *testing.T) {
	sessionCode := "AP-7K9X-M2NF"
	resumeTime := time.Date(2026, time.March, 13, 14, 0, 0, 0, time.UTC)
	issuedAt := resumeTime.Add(-2 * time.Hour)

	store := newQAServiceStore()
	store.getAnswerCountFn = func(context.Context, string) (int, error) {
		return 3, nil
	}
	store.getFlowStateFn = func(context.Context, string) (*FlowState, error) {
		return &FlowState{
			Step:           FlowStepCriterion,
			ExpectedTurnID: "turn-criterion",
			QuestionNumber: 4,
			ActiveQuestion: &IssuedQuestion{
				Question: Question{
					TextEs:         "Que paso despues?",
					TextEn:         "What happened next?",
					Area:           "protected_ground",
					Kind:           QuestionKindCriterion,
					TurnID:         "turn-criterion",
					QuestionNumber: 4,
					TotalQuestions: EstimatedTotalQuestions,
				},
				IssuedAt:         issuedAt,
				AnswerDeadlineAt: issuedAt.Add(5 * time.Minute),
			},
		}, nil
	}

	sessions := &fakeInterviewSessionStore{
		getSessionByCodeFn: func(context.Context, string) (*session.Session, error) {
			return &session.Session{
				SessionCode:            sessionCode,
				PreferredLanguage:      "en",
				Status:                 "interviewing",
				InterviewBudgetSeconds: 2400,
				InterviewLapsedSeconds: 600,
				ExpiresAt:              resumeTime.Add(24 * time.Hour),
			}, nil
		},
		startSessionFn: func(_ context.Context, _, _ string) (*session.Session, error) {
			return &session.Session{
				SessionCode:            sessionCode,
				PreferredLanguage:      "en",
				Status:                 "interviewing",
				InterviewBudgetSeconds: 2400,
				InterviewLapsedSeconds: 600,
				ExpiresAt:              resumeTime.Add(24 * time.Hour),
			}, nil
		},
	}

	svc := newServiceForRecoveryTests(store, sessions, &qaAIClient{})
	svc.nowFn = func() time.Time { return resumeTime }

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
	if result.Question.TurnID != "turn-criterion" {
		t.Fatalf("question.turnID = %q, want turn-criterion", result.Question.TurnID)
	}
	if result.TimerRemainingS != 1500 {
		t.Fatalf("timerRemainingS = %d, want 1500 after capping the 2-hour offline gap to one question limit", result.TimerRemainingS)
	}
	if result.AnswerSubmitWindowRemainingS != 0 {
		t.Fatalf("answerSubmitWindowRemainingS = %d, want 0 after the per-question deadline passed", result.AnswerSubmitWindowRemainingS)
	}
}

func TestBuildStartIssuePlan_SelectsQuestionByResumeState(t *testing.T) {
	svc := newServiceForRecoveryTests(newQAServiceStore(), nil, &qaAIClient{})

	freshPlan, err := svc.buildStartIssuePlan(&startInterviewState{
		flowState: &FlowState{QuestionNumber: 1},
		resuming:  false,
	}, "protected_ground")
	if err != nil {
		t.Fatalf("buildStartIssuePlan() fresh error = %v", err)
	}
	if freshPlan.resuming {
		t.Fatalf("fresh plan resuming = %v, want false", freshPlan.resuming)
	}
	if freshPlan.issue.question == nil {
		t.Fatalf("fresh question = nil, want non-nil")
	}
	if freshPlan.issue.question.Kind != QuestionKindDisclaimer {
		t.Fatalf("fresh question.kind = %q, want %q", freshPlan.issue.question.Kind, QuestionKindDisclaimer)
	}
	if freshPlan.issue.question.TextEn != "Opening disclaimer EN" {
		t.Fatalf("fresh question.textEn = %q, want opening disclaimer", freshPlan.issue.question.TextEn)
	}

	resumePlan, err := svc.buildStartIssuePlan(&startInterviewState{
		flowState: &FlowState{QuestionNumber: 6},
		resuming:  true,
	}, "protected_ground")
	if err != nil {
		t.Fatalf("buildStartIssuePlan() resume error = %v", err)
	}
	if !resumePlan.resuming {
		t.Fatalf("resume plan resuming = %v, want true", resumePlan.resuming)
	}
	if resumePlan.issue.question == nil {
		t.Fatalf("resume question = nil, want non-nil")
	}
	if resumePlan.issue.question.Kind != QuestionKindReadiness {
		t.Fatalf("resume question.kind = %q, want %q", resumePlan.issue.question.Kind, QuestionKindReadiness)
	}
	if resumePlan.issue.question.TextEn != ResumeQuestion("protected_ground").TextEn {
		t.Fatalf("resume question.textEn = %q, want %q", resumePlan.issue.question.TextEn, ResumeQuestion("protected_ground").TextEn)
	}
}

func TestBuildDisclaimerAdvancePlan_PreservesReadinessTextByQuestionNumber(t *testing.T) {
	svc := newServiceForRecoveryTests(newQAServiceStore(), nil, &qaAIClient{})

	initialPlan, err := svc.buildDisclaimerAdvancePlan(&turnSnapshot{
		currentArea: &QuestionArea{Area: "protected_ground"},
		flowState:   &FlowState{QuestionNumber: 1},
	})
	if err != nil {
		t.Fatalf("buildDisclaimerAdvancePlan() initial error = %v", err)
	}
	if initialPlan.issue.question == nil {
		t.Fatalf("initial question = nil, want non-nil")
	}
	if initialPlan.issue.question.Kind != QuestionKindReadiness {
		t.Fatalf("initial question.kind = %q, want %q", initialPlan.issue.question.Kind, QuestionKindReadiness)
	}
	if initialPlan.issue.question.TextEn != svc.settings.ReadinessQuestion.En {
		t.Fatalf("initial question.textEn = %q, want %q", initialPlan.issue.question.TextEn, svc.settings.ReadinessQuestion.En)
	}

	resumePlan, err := svc.buildDisclaimerAdvancePlan(&turnSnapshot{
		currentArea: &QuestionArea{Area: "protected_ground"},
		flowState:   &FlowState{QuestionNumber: 3},
	})
	if err != nil {
		t.Fatalf("buildDisclaimerAdvancePlan() resume error = %v", err)
	}
	if resumePlan.issue.question == nil {
		t.Fatalf("resume question = nil, want non-nil")
	}
	if resumePlan.issue.question.TextEn != ResumeQuestion("protected_ground").TextEn {
		t.Fatalf("resume question.textEn = %q, want %q", resumePlan.issue.question.TextEn, ResumeQuestion("protected_ground").TextEn)
	}
}

func TestIssuedQuestionResultData_PrefersIssuedQuestionAndCarriesFlags(t *testing.T) {
	svc := newServiceForRecoveryTests(newQAServiceStore(), nil, &qaAIClient{})
	now := time.Date(2026, time.March, 13, 14, 0, 0, 0, time.UTC)
	svc.nowFn = func() time.Time { return now }

	fallbackQuestion := &Question{
		TextEs:         "Fallback",
		TextEn:         "Fallback",
		Area:           "protected_ground",
		Kind:           QuestionKindCriterion,
		TurnID:         "fallback-turn",
		QuestionNumber: 3,
		TotalQuestions: EstimatedTotalQuestions,
	}
	issuedQuestion := &IssuedQuestion{
		Question: Question{
			TextEs:         "Persisted",
			TextEn:         "Persisted",
			Area:           "protected_ground",
			Kind:           QuestionKindCriterion,
			TurnID:         "persisted-turn",
			QuestionNumber: 4,
			TotalQuestions: EstimatedTotalQuestions,
		},
		IssuedAt:         now,
		AnswerDeadlineAt: now.Add(4 * time.Minute),
	}

	data := svc.issuedQuestionResultData(issuedQuestion, questionIssue{
		question:    fallbackQuestion,
		area:        fallbackQuestion.Area,
		substituted: true,
	})
	if data.question == nil {
		t.Fatalf("question = nil, want non-nil")
	}
	if data.question.TurnID != "persisted-turn" {
		t.Fatalf("question.turnID = %q, want persisted-turn", data.question.TurnID)
	}
	if data.area != "protected_ground" {
		t.Fatalf("area = %q, want protected_ground", data.area)
	}
	if data.answerSubmitWindowRemainingS != 240 {
		t.Fatalf("answerSubmitWindowRemainingS = %d, want 240", data.answerSubmitWindowRemainingS)
	}
	if !data.substituted {
		t.Fatalf("substituted = %v, want true", data.substituted)
	}
}

func TestProcessTurn_ReadinessStepTriggersNewAICallAndReturnsNewQuestion(t *testing.T) {
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
		return &FlowState{
			Step:           FlowStepCriterion,
			ExpectedTurnID: params.NextIssuedQuestion.Question.TurnID,
			QuestionNumber: 3,
			ActiveQuestion: params.NextIssuedQuestion,
		}, nil
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
		generateTurnFn: func(_ context.Context, _ *AITurnContext) (*AIResponse, error) {
			return &AIResponse{NextQuestion: "Please tell me about your first entry."}, nil
		},
	}

	svc := newServiceForRecoveryTests(store, sessions, ai)

	result, err := svc.processTurn(context.Background(), sessionCode, "Yes", "Ready", "turn-readiness")
	if err != nil {
		t.Fatalf("processTurn() error = %v", err)
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

func TestLoadReadinessOpeningInputs_BuildsOpeningTurnContext(t *testing.T) {
	sessionCode := "AP-7K9X-M2NF"
	store := newQAServiceStore()
	store.getAnswersBySessionFn = func(context.Context, string) ([]Answer, error) {
		return []Answer{
			{
				Area:         "protected_ground",
				QuestionText: "Prior question",
				TranscriptEn: "Prior answer",
				TranscriptEs: "Respuesta previa",
			},
		}, nil
	}

	svc := newServiceForRecoveryTests(store, nil, &qaAIClient{})
	snapshot := &turnSnapshot{
		session:           activeSession(sessionCode, "en"),
		areas:             []QuestionArea{{Area: "protected_ground", Status: AreaStatusInProgress, QuestionsCount: 1}},
		currentArea:       &QuestionArea{Area: "protected_ground", Status: AreaStatusInProgress, QuestionsCount: 1},
		flowState:         &FlowState{QuestionNumber: 2},
		preferredLanguage: "en",
		timeRemainingS:    1200,
	}

	inputs, err := svc.loadReadinessOpeningInputs(context.Background(), sessionCode, snapshot)
	if err != nil {
		t.Fatalf("loadReadinessOpeningInputs() error = %v", err)
	}
	if len(inputs.answers) != 1 {
		t.Fatalf("answers length = %d, want 1", len(inputs.answers))
	}
	if inputs.areaCfg.Slug != "protected_ground" {
		t.Fatalf("areaCfg.slug = %q, want protected_ground", inputs.areaCfg.Slug)
	}
	if inputs.areaIndex != 0 {
		t.Fatalf("areaIndex = %d, want 0", inputs.areaIndex)
	}
	if inputs.fallbackQuestion != "Fallback protected ground question" {
		t.Fatalf("fallbackQuestion = %q, want fallback question", inputs.fallbackQuestion)
	}
	if inputs.turnCtx == nil {
		t.Fatalf("turnCtx = nil, want non-nil")
	}
	if !inputs.turnCtx.IsOpeningTurn {
		t.Fatalf("turnCtx.isOpeningTurn = %v, want true", inputs.turnCtx.IsOpeningTurn)
	}
	if inputs.turnCtx.CurrentAreaSlug != "protected_ground" {
		t.Fatalf("turnCtx.currentAreaSlug = %q, want protected_ground", inputs.turnCtx.CurrentAreaSlug)
	}
	if inputs.turnCtx.HistoryTurns[0].AnswerText != "Prior answer" {
		t.Fatalf("turnCtx.historyTurns[0].answerText = %q, want Prior answer", inputs.turnCtx.HistoryTurns[0].AnswerText)
	}
}

func TestSelectReadinessOpeningQuestion_UsesAIQuestion(t *testing.T) {
	ai := &qaAIClient{
		generateTurnFn: func(_ context.Context, _ *AITurnContext) (*AIResponse, error) {
			return &AIResponse{NextQuestion: "Please tell me more."}, nil
		},
	}
	svc := newServiceForRecoveryTests(newQAServiceStore(), nil, ai)
	selection, err := svc.selectReadinessOpeningQuestion(context.Background(), "AP-7K9X-M2NF", &turnSnapshot{
		currentArea: &QuestionArea{Area: "protected_ground"},
	}, &readinessOpeningInputs{
		fallbackQuestion: "Fallback protected ground question",
		turnCtx:          &AITurnContext{CurrentAreaSlug: "protected_ground"},
	})
	if err != nil {
		t.Fatalf("selectReadinessOpeningQuestion() error = %v", err)
	}
	if selection.questionText != "Please tell me more." {
		t.Fatalf("questionText = %q, want AI question", selection.questionText)
	}
	if selection.substituted {
		t.Fatalf("substituted = %v, want false", selection.substituted)
	}
}

func TestSelectReadinessOpeningQuestion_RetryExhaustedFallsBack(t *testing.T) {
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
	selection, err := svc.selectReadinessOpeningQuestion(context.Background(), "AP-7K9X-M2NF", &turnSnapshot{
		currentArea: &QuestionArea{Area: "protected_ground"},
	}, &readinessOpeningInputs{
		fallbackQuestion: "Fallback protected ground question",
		turnCtx:          &AITurnContext{CurrentAreaSlug: "protected_ground"},
	})
	if err != nil {
		t.Fatalf("selectReadinessOpeningQuestion() error = %v", err)
	}
	if selection.questionText != "Fallback protected ground question" {
		t.Fatalf("questionText = %q, want fallback question", selection.questionText)
	}
	if !selection.substituted {
		t.Fatalf("substituted = %v, want true", selection.substituted)
	}
}

func TestSelectReadinessOpeningQuestion_EmptyAIQuestionFallsBack(t *testing.T) {
	ai := &qaAIClient{
		generateTurnFn: func(_ context.Context, _ *AITurnContext) (*AIResponse, error) {
			return &AIResponse{NextQuestion: "   "}, nil
		},
	}
	svc := newServiceForRecoveryTests(newQAServiceStore(), nil, ai)
	selection, err := svc.selectReadinessOpeningQuestion(context.Background(), "AP-7K9X-M2NF", &turnSnapshot{
		currentArea: &QuestionArea{Area: "protected_ground"},
	}, &readinessOpeningInputs{
		fallbackQuestion: "Fallback protected ground question",
		turnCtx:          &AITurnContext{CurrentAreaSlug: "protected_ground"},
	})
	if err != nil {
		t.Fatalf("selectReadinessOpeningQuestion() error = %v", err)
	}
	if selection.questionText != "Fallback protected ground question" {
		t.Fatalf("questionText = %q, want fallback question", selection.questionText)
	}
	if !selection.substituted {
		t.Fatalf("substituted = %v, want true", selection.substituted)
	}
}

func TestSelectReadinessOpeningQuestion_AbortedRetryPropagatesError(t *testing.T) {
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

	_, err := svc.selectReadinessOpeningQuestion(ctx, "AP-7K9X-M2NF", &turnSnapshot{
		currentArea: &QuestionArea{Area: "protected_ground"},
	}, &readinessOpeningInputs{
		fallbackQuestion: "Fallback protected ground question",
		turnCtx:          &AITurnContext{CurrentAreaSlug: "protected_ground"},
	})
	if err == nil {
		t.Fatalf("selectReadinessOpeningQuestion() error = nil, want non-nil")
	}
	if got := err.Error(); got != "AI retry aborted: context canceled" {
		t.Fatalf("error = %q, want AI retry aborted: context canceled", got)
	}
}

func TestBuildReadinessAdvancePlan_MapsSelectionToCriterionQuestion(t *testing.T) {
	svc := newServiceForRecoveryTests(newQAServiceStore(), nil, &qaAIClient{})

	plan, err := svc.buildReadinessAdvancePlan(&turnSnapshot{
		currentArea: &QuestionArea{Area: "protected_ground"},
		flowState:   &FlowState{QuestionNumber: 2},
	}, &readinessOpeningSelection{
		questionText: "Please tell me more.",
		substituted:  true,
	})
	if err != nil {
		t.Fatalf("buildReadinessAdvancePlan() error = %v", err)
	}
	if plan.issue.question == nil {
		t.Fatalf("plan.issue.question = nil, want non-nil")
	}
	if plan.issue.question.Kind != QuestionKindCriterion {
		t.Fatalf("plan.issue.question.kind = %q, want %q", plan.issue.question.Kind, QuestionKindCriterion)
	}
	if plan.issue.question.Area != "protected_ground" {
		t.Fatalf("plan.issue.question.area = %q, want protected_ground", plan.issue.question.Area)
	}
	if plan.issue.question.QuestionNumber != 3 {
		t.Fatalf("plan.issue.question.questionNumber = %d, want 3", plan.issue.question.QuestionNumber)
	}
	if !plan.issue.substituted {
		t.Fatalf("plan.issue.substituted = %v, want true", plan.issue.substituted)
	}
	if plan.issue.question.TurnID == "" {
		t.Fatalf("plan.issue.question.turnID = empty, want generated turn ID")
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

func TestProcessTurn_CriterionStepCompletesInterviewAcrossSessionLanguages(t *testing.T) {
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
				return &ProcessCriterionTurnResult{NewCount: 2}, nil
			}

			sessions := &fakeInterviewSessionStore{
				getSessionByCodeFn: func(context.Context, string) (*session.Session, error) {
					return activeSession(sessionCode, tc.sessionLanguage), nil
				},
			}

			ai := &qaAIClient{
				generateTurnFn: func(context.Context, *AITurnContext) (*AIResponse, error) {
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

			result, err := svc.processTurn(context.Background(), sessionCode, answerText, questionText, turnID)
			if err != nil {
				t.Fatalf("processTurn() error = %v", err)
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
