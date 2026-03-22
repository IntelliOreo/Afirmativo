package interview

import (
	"context"
	"time"

	"github.com/afirmativo/backend/internal/config"
	"github.com/afirmativo/backend/internal/session"
)

type fakeInterviewStore struct {
	upsertAnswerJobFn               func(ctx context.Context, params UpsertAnswerJobParams) (*AnswerJob, error)
	getAnswerJobFn                  func(ctx context.Context, sessionCode, jobID string) (*AnswerJob, error)
	claimQueuedAnswerJobFn          func(ctx context.Context, jobID string) (*AnswerJob, error)
	claimNextQueuedAnswerJobFn      func(ctx context.Context) (*AnswerJob, error)
	requeueStaleRunningAnswerJobsFn func(ctx context.Context, staleBefore time.Time) (int64, error)
	processCriterionTurnFn          func(ctx context.Context, params ProcessCriterionTurnParams) (*ProcessCriterionTurnResult, error)
	markAreaNotAssessedFn           func(ctx context.Context, sessionCode, area string) error
	markFlowDoneFn                  func(ctx context.Context, sessionCode string) error
	markAnswerJobOKFn               func(ctx context.Context, jobID string, resultPayload []byte) error
	markAnswerJobFailedFn           func(ctx context.Context, params MarkAnswerJobFailedParams) error
}

type fakeInterviewSessionStore struct {
	startSessionFn     func(ctx context.Context, sessionCode, preferredLanguage string) (*session.Session, error)
	getSessionByCodeFn func(ctx context.Context, sessionCode string) (*session.Session, error)
	completeSessionFn  func(ctx context.Context, sessionCode string) error
}

func (f *fakeInterviewStore) CreateQuestionArea(context.Context, string, string) (*QuestionArea, error) {
	return nil, nil
}
func (f *fakeInterviewStore) SetAreaInProgress(context.Context, string, string) error { return nil }
func (f *fakeInterviewStore) GetInProgressArea(context.Context, string) (*QuestionArea, error) {
	return nil, nil
}
func (f *fakeInterviewStore) GetAreasBySession(context.Context, string) ([]QuestionArea, error) {
	return nil, nil
}
func (f *fakeInterviewStore) IncrementAreaQuestions(context.Context, string, string) error {
	return nil
}
func (f *fakeInterviewStore) CompleteArea(context.Context, string, string) error         { return nil }
func (f *fakeInterviewStore) MarkAreaInsufficient(context.Context, string, string) error { return nil }
func (f *fakeInterviewStore) MarkAreaPreAddressed(context.Context, string, string, string) error {
	return nil
}
func (f *fakeInterviewStore) MarkAreaNotAssessed(ctx context.Context, sessionCode, area string) error {
	if f.markAreaNotAssessedFn != nil {
		return f.markAreaNotAssessedFn(ctx, sessionCode, area)
	}
	return nil
}
func (f *fakeInterviewStore) SaveAnswer(context.Context, SaveAnswerParams) (*Answer, error) {
	return nil, nil
}
func (f *fakeInterviewStore) GetAnswersBySession(context.Context, string) ([]Answer, error) {
	return nil, nil
}
func (f *fakeInterviewStore) GetAnswerCount(context.Context, string) (int, error) { return 0, nil }
func (f *fakeInterviewStore) GetFlowState(context.Context, string) (*FlowState, error) {
	return nil, nil
}
func (f *fakeInterviewStore) PrepareDisclaimerStep(context.Context, string, *IssuedQuestion) (*FlowState, error) {
	return nil, nil
}
func (f *fakeInterviewStore) PrepareReadinessStep(context.Context, string, *IssuedQuestion) (*FlowState, error) {
	return nil, nil
}
func (f *fakeInterviewStore) AdvanceNonCriterionStep(context.Context, AdvanceNonCriterionStepParams) (*FlowState, error) {
	return nil, nil
}
func (f *fakeInterviewStore) ProcessCriterionTurn(ctx context.Context, params ProcessCriterionTurnParams) (*ProcessCriterionTurnResult, error) {
	if f.processCriterionTurnFn != nil {
		return f.processCriterionTurnFn(ctx, params)
	}
	return nil, nil
}
func (f *fakeInterviewStore) MarkFlowDone(ctx context.Context, sessionCode string) error {
	if f.markFlowDoneFn != nil {
		return f.markFlowDoneFn(ctx, sessionCode)
	}
	return nil
}

func (f *fakeInterviewStore) UpsertAnswerJob(ctx context.Context, params UpsertAnswerJobParams) (*AnswerJob, error) {
	if f.upsertAnswerJobFn != nil {
		return f.upsertAnswerJobFn(ctx, params)
	}
	return nil, nil
}

func (f *fakeInterviewStore) ClaimQueuedAnswerJob(ctx context.Context, jobID string) (*AnswerJob, error) {
	if f.claimQueuedAnswerJobFn != nil {
		return f.claimQueuedAnswerJobFn(ctx, jobID)
	}
	return nil, nil
}

func (f *fakeInterviewStore) ClaimNextQueuedAnswerJob(ctx context.Context) (*AnswerJob, error) {
	if f.claimNextQueuedAnswerJobFn != nil {
		return f.claimNextQueuedAnswerJobFn(ctx)
	}
	return nil, nil
}

func (f *fakeInterviewStore) RequeueStaleRunningAnswerJobs(ctx context.Context, staleBefore time.Time) (int64, error) {
	if f.requeueStaleRunningAnswerJobsFn != nil {
		return f.requeueStaleRunningAnswerJobsFn(ctx, staleBefore)
	}
	return 0, nil
}

func (f *fakeInterviewStore) GetAnswerJob(ctx context.Context, sessionCode, jobID string) (*AnswerJob, error) {
	if f.getAnswerJobFn != nil {
		return f.getAnswerJobFn(ctx, sessionCode, jobID)
	}
	return nil, nil
}

func (f *fakeInterviewStore) MarkAnswerJobSucceeded(ctx context.Context, jobID string, resultPayload []byte) error {
	if f.markAnswerJobOKFn != nil {
		return f.markAnswerJobOKFn(ctx, jobID, resultPayload)
	}
	return nil
}

func (f *fakeInterviewStore) MarkAnswerJobFailed(ctx context.Context, params MarkAnswerJobFailedParams) error {
	if f.markAnswerJobFailedFn != nil {
		return f.markAnswerJobFailedFn(ctx, params)
	}
	return nil
}

func (f *fakeInterviewStore) AppendAnswerJobFailedReason(context.Context, string, string) error {
	return nil
}

func (f *fakeInterviewStore) IncrementAnswerJobAttempts(context.Context, string) error {
	return nil
}

func defaultInterviewSettings(asyncRuntime ...config.AsyncRuntimeConfig) Settings {
	runtime := config.AsyncRuntimeConfig{
		Workers:       4,
		QueueSize:     256,
		RecoveryEvery: 10 * time.Second,
		StaleAfter:    3 * time.Minute,
		JobTimeout:    3 * time.Minute,
	}
	if len(asyncRuntime) > 0 {
		runtime = asyncRuntime[0]
	}
	return Settings{
		AreaConfigs: []config.AreaConfig{
			{
				ID:                      1,
				Slug:                    "protected_ground",
				Label:                   "Protected ground",
				Description:             "Protected ground description",
				SufficiencyRequirements: "Protected ground sufficiency requirements",
				FallbackQuestion:        "Fallback protected ground question",
			},
		},
		OpeningDisclaimer:      config.BilingualText{En: "Opening disclaimer EN", Es: "Opening disclaimer ES"},
		ReadinessQuestion:      config.BilingualText{En: "Default readiness EN", Es: "Default readiness ES"},
		AnswerTimeLimitSeconds: 300,
		DBTimeout:              5 * time.Second,
		AsyncRuntime:           runtime,
	}
}

func newInterviewServiceForAsyncTests(store Store, asyncRuntime ...config.AsyncRuntimeConfig) *Service {
	fakeSessions := &fakeInterviewSessionStore{}
	return NewService(Deps{
		SessionStarter:   fakeSessions,
		SessionGetter:    fakeSessions,
		SessionCompleter: fakeSessions,
		Store:            store,
	}, defaultInterviewSettings(asyncRuntime...))
}

func (f *fakeInterviewSessionStore) StartSession(ctx context.Context, sessionCode, preferredLanguage string) (*session.Session, error) {
	if f.startSessionFn != nil {
		return f.startSessionFn(ctx, sessionCode, preferredLanguage)
	}
	now := time.Now().UTC()
	return &session.Session{
		SessionCode:               sessionCode,
		PreferredLanguage:         preferredLanguage,
		Status:                    "interviewing",
		InterviewBudgetSeconds:    3600,
		InterviewLapsedSeconds:    0,
		CurrentInterviewStartedAt: &now,
		ExpiresAt:                 now.Add(24 * time.Hour),
	}, nil
}

func (f *fakeInterviewSessionStore) GetSessionByCode(ctx context.Context, sessionCode string) (*session.Session, error) {
	if f.getSessionByCodeFn != nil {
		return f.getSessionByCodeFn(ctx, sessionCode)
	}
	return &session.Session{
		SessionCode: sessionCode,
		Status:      "interviewing",
		ExpiresAt:   time.Now().UTC().Add(24 * time.Hour),
	}, nil
}

func (f *fakeInterviewSessionStore) CompleteSession(ctx context.Context, sessionCode string) error {
	if f.completeSessionFn != nil {
		return f.completeSessionFn(ctx, sessionCode)
	}
	return nil
}
