// Service layer for interview operations.
// StartInterview: sets session to interviewing, creates areas, returns first question.
// processTurn: persists answer, evaluates via AI, manages area transitions.
package interview

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/afirmativo/backend/internal/config"
	"github.com/afirmativo/backend/internal/session"
)

// SessionStarter transitions a session to 'interviewing'.
type SessionStarter interface {
	StartSession(ctx context.Context, sessionCode, preferredLanguage string) (*session.Session, error)
}

// SessionGetter retrieves session data (for timer calculation).
type SessionGetter interface {
	GetSessionByCode(ctx context.Context, sessionCode string) (*session.Session, error)
}

// SessionCompleter marks a session as completed.
type SessionCompleter interface {
	CompleteSession(ctx context.Context, sessionCode string) error
}

// InterviewAIClient calls the AI API to evaluate answers and generate next questions.
type InterviewAIClient interface {
	GenerateTurn(ctx context.Context, turnCtx *AITurnContext) (*AIResponse, error)
}

type interviewStateStore = InterviewStateStore
type asyncAnswerJobStore = AsyncAnswerJobStore

// Service contains interview business logic.
type Service struct {
	sessionStarter   SessionStarter
	sessionGetter    SessionGetter
	sessionCompleter SessionCompleter
	stateStore       interviewStateStore
	jobStore         asyncAnswerJobStore
	aiClient         InterviewAIClient
	settings         Settings
	nowFn            func() time.Time
	dbTimeout        time.Duration

	asyncAnswerWorkers       int
	asyncAnswerRecoveryBatch int
	asyncAnswerRecoveryEvery time.Duration
	asyncAnswerStaleAfter    time.Duration
	asyncAnswerJobTimeout    time.Duration
	asyncAnswerQueue         chan string
	asyncRuntimeStartOnce    sync.Once
	asyncAnswerRequestIDs    sync.Map
}

type Deps struct {
	SessionStarter   SessionStarter
	SessionGetter    SessionGetter
	SessionCompleter SessionCompleter
	Store            Store
	AIClient         InterviewAIClient
}

type Settings struct {
	AreaConfigs            []config.AreaConfig
	OpeningDisclaimer      config.BilingualText
	ReadinessQuestion      config.BilingualText
	AnswerTimeLimitSeconds int
	DBTimeout              time.Duration
	AsyncRuntime           config.AsyncRuntimeConfig
}

// NewService creates a Service with the given dependencies.
func NewService(deps Deps, settings Settings) *Service {
	svc := &Service{
		sessionStarter:           deps.SessionStarter,
		sessionGetter:            deps.SessionGetter,
		sessionCompleter:         deps.SessionCompleter,
		stateStore:               deps.Store,
		jobStore:                 deps.Store,
		aiClient:                 deps.AIClient,
		settings:                 settings,
		nowFn:                    time.Now,
		dbTimeout:                settings.DBTimeout,
		asyncAnswerWorkers:       settings.AsyncRuntime.Workers,
		asyncAnswerRecoveryBatch: settings.AsyncRuntime.RecoveryBatch,
		asyncAnswerRecoveryEvery: settings.AsyncRuntime.RecoveryEvery,
		asyncAnswerStaleAfter:    settings.AsyncRuntime.StaleAfter,
		asyncAnswerJobTimeout:    settings.AsyncRuntime.JobTimeout,
	}
	svc.asyncAnswerQueue = make(chan string, settings.AsyncRuntime.QueueSize)
	return svc
}

// StartResult holds the output of a successful interview start.
type StartResult struct {
	Question                     *Question
	TimerRemainingS              int
	AnswerSubmitWindowRemainingS int
	Area                         string
	Resuming                     bool
	Language                     string
}

// AnswerResult holds the output of a submitted answer.
type AnswerResult struct {
	Done                         bool
	NextQuestion                 *Question
	TimerRemainingS              int
	AnswerSubmitWindowRemainingS int
	Substituted                  bool
}

// processTurn processes one answer according to the explicit flow step.
func (s *Service) processTurn(ctx context.Context, sessionCode, answerText, questionText, turnID string) (*AnswerResult, error) {
	return s.processTurnCore(ctx, sessionCode, answerText, questionText, turnID, s.nowFn(), nil)
}

func (s *Service) processTurnForAsyncJob(ctx context.Context, job *AnswerJob) (*AnswerResult, error) {
	return s.processTurnCore(
		ctx,
		job.SessionCode,
		job.AnswerText,
		job.QuestionText,
		job.TurnID,
		job.CreatedAt,
		s.newAsyncJobRetryFailureRecorder(job.ID),
	)
}

func (s *Service) processTurnCore(
	ctx context.Context,
	sessionCode, answerText, questionText, turnID string,
	submissionTime time.Time,
	failureRecorder aiRetryFailureRecorder,
) (*AnswerResult, error) {
	snapshot, err := s.buildTurnSnapshot(
		ctx,
		sessionCode,
		answerText,
		questionText,
		turnID,
		submissionTime,
		failureRecorder,
	)
	if err != nil {
		return nil, err
	}

	if snapshot.flowState.Step == FlowStepDone {
		s.finishSession(ctx, sessionCode)
		return &AnswerResult{Done: true, TimerRemainingS: 0, AnswerSubmitWindowRemainingS: 0}, nil
	}
	if strings.TrimSpace(snapshot.turnID) == "" || snapshot.turnID != snapshot.flowState.ExpectedTurnID {
		return nil, ErrTurnConflict
	}
	if snapshot.timeRemainingS <= 0 {
		return s.finishOnTimeout(ctx, sessionCode, snapshot.areas)
	}

	switch snapshot.flowState.Step {
	case FlowStepDisclaimer:
		return s.handleDisclaimerTurn(ctx, sessionCode, snapshot)
	case FlowStepReadiness:
		return s.handleReadinessTurn(ctx, sessionCode, snapshot)
	case FlowStepCriterion:
		return s.handleCriterionTurn(ctx, sessionCode, snapshot)
	default:
		return nil, ErrInvalidFlow
	}
}

// finishSession marks the session as completed. Logs on error but does not
// propagate - the interview result has already been determined.
func (s *Service) finishSession(ctx context.Context, sessionCode string) {
	if err := s.sessionCompleter.CompleteSession(ctx, sessionCode); err != nil {
		slog.Error("failed to complete session", "session", sessionCode, "error", err)
	}
}

func (s *Service) finishOnTimeout(ctx context.Context, sessionCode string, areas []QuestionArea) (*AnswerResult, error) {
	s.markRemainingNotAssessed(ctx, sessionCode, areas)
	if err := s.stateStore.MarkFlowDone(ctx, sessionCode); err != nil {
		slog.Warn("failed to mark flow done on timeout", "session", sessionCode, "error", err)
	}
	s.finishSession(ctx, sessionCode)
	return &AnswerResult{Done: true, TimerRemainingS: 0, AnswerSubmitWindowRemainingS: 0}, nil
}

func (s *Service) finishIfNoCurrentArea(ctx context.Context, sessionCode string, currentArea *QuestionArea, markDone bool) bool {
	if currentArea != nil {
		return false
	}
	if markDone {
		if err := s.stateStore.MarkFlowDone(ctx, sessionCode); err != nil {
			slog.Warn("failed to mark flow done with no current area", "session", sessionCode, "error", err)
		}
	}
	s.finishSession(ctx, sessionCode)
	return true
}

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

func (s *Service) orderedAreaSlugs() []string {
	slugs := make([]string, 0, len(s.settings.AreaConfigs))
	for _, cfg := range s.settings.AreaConfigs {
		slugs = append(slugs, cfg.Slug)
	}
	return slugs
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

func (s *Service) extractPreAddressed(other []OtherCriterion) []PreAddressedArea {
	flags := make([]PreAddressedArea, 0, len(other))
	for _, item := range other {
		slug := s.matchAreaSlug(item.Name)
		if slug == "" {
			slog.Warn("cross-criteria flag: no matching area", "name", item.Name)
			continue
		}
		flags = append(flags, PreAddressedArea{
			Slug:     slug,
			Evidence: item.EvidenceSummary,
		})
	}
	return flags
}

func (s *Service) findAreaConfig(slug string) (config.AreaConfig, int) {
	for i, ac := range s.settings.AreaConfigs {
		if ac.Slug == slug {
			return ac, i
		}
	}
	// Return a minimal config if not found (shouldn't happen in practice).
	return config.AreaConfig{Slug: slug, Label: slug}, -1
}

func (s *Service) buildCriteriaCoverage(areas []QuestionArea) []CriteriaCoverage {
	coverage := make([]CriteriaCoverage, 0, len(areas))
	for _, a := range areas {
		cfg, _ := s.findAreaConfig(a.Area)
		coverage = append(coverage, CriteriaCoverage{
			ID:     cfg.ID,
			Name:   a.Area,
			Status: a.Status,
		})
	}
	return coverage
}

func (s *Service) buildHistoryTurns(answers []Answer, preferredLanguage string) []HistoryTurn {
	useEnglish := strings.EqualFold(strings.TrimSpace(preferredLanguage), "en")

	historyTurns := make([]HistoryTurn, 0, len(answers))
	for _, a := range answers {
		historyTurns = append(historyTurns, HistoryTurn{
			QuestionText: a.QuestionText,
			AnswerText:   selectTranscript(useEnglish, a.TranscriptEn, a.TranscriptEs),
		})
	}
	return historyTurns
}

func (s *Service) countCriteriaRemaining(areas []QuestionArea) int {
	count := 0
	for _, a := range areas {
		if isAreaUnresolved(a.Status) {
			count++
		}
	}
	return count
}

func (s *Service) buildAITurnContext(
	area QuestionArea,
	areaCfg config.AreaConfig,
	areaIndex int,
	answers []Answer,
	areas []QuestionArea,
	preferredLanguage string,
	totalBudgetS int,
	timeRemainingS int,
	isOpening bool,
	questionText string,
	answerText string,
) *AITurnContext {
	return &AITurnContext{
		PreferredLanguage:   preferredLanguage,
		CurrentAreaSlug:     area.Area,
		CurrentAreaID:       areaCfg.ID,
		CurrentAreaIndex:    areaIndex,
		IsOpeningTurn:       isOpening,
		CurrentQuestionText: questionText,
		LatestAnswerText:    answerText,
		CurrentAreaLabel:    areaCfg.Label,
		Description:         areaCfg.Description,
		SufficiencyReqs:     areaCfg.SufficiencyRequirements,
		AreaStatus:          area.Status,
		IsPreAddressed:      area.Status == AreaStatusPreAddressed,
		FollowUpsRemaining:  MaxQuestionsPerArea - area.QuestionsCount,
		TotalBudgetS:        totalBudgetS,
		TimeRemainingS:      timeRemainingS,
		QuestionsRemaining:  EstimatedTotalQuestions - len(answers),
		CriteriaRemaining:   s.countCriteriaRemaining(areas),
		CriteriaCoverage:    s.buildCriteriaCoverage(areas),
		HistoryTurns:        s.buildHistoryTurns(answers, preferredLanguage),
	}
}

// matchAreaSlug tries to find a matching area slug from the AI's cross-criteria name.
// Uses case-insensitive matching against both slugs and labels.
func (s *Service) matchAreaSlug(name string) string {
	lower := strings.ToLower(name)
	for _, ac := range s.settings.AreaConfigs {
		if strings.ToLower(ac.Slug) == lower || strings.ToLower(ac.Label) == lower {
			return ac.Slug
		}
	}
	return ""
}

func (s *Service) markRemainingNotAssessed(ctx context.Context, sessionCode string, areas []QuestionArea) {
	dbCtx, dbCancel := context.WithTimeout(ctx, s.dbTimeout)
	defer dbCancel()
	for _, a := range areas {
		if isAreaUnresolved(a.Status) {
			if err := s.stateStore.MarkAreaNotAssessed(dbCtx, sessionCode, a.Area); err != nil {
				slog.Warn("failed to mark not_assessed", "area", a.Area, "error", err)
			}
		}
	}
}

func selectTranscript(preferEnglish bool, en, es string) string {
	if preferEnglish {
		if candidate := strings.TrimSpace(en); candidate != "" {
			return en
		}
		return es
	}
	if candidate := strings.TrimSpace(es); candidate != "" {
		return es
	}
	return en
}

func (s *Service) refreshAreaState(ctx context.Context, sessionCode string) ([]QuestionArea, *QuestionArea, error) {
	dbCtx, dbCancel := context.WithTimeout(ctx, s.dbTimeout)
	defer dbCancel()

	areas, err := s.stateStore.GetAreasBySession(dbCtx, sessionCode)
	if err != nil {
		return nil, nil, fmt.Errorf("get areas by session: %w", err)
	}

	currentArea, err := s.stateStore.GetInProgressArea(dbCtx, sessionCode)
	if err != nil {
		return nil, nil, fmt.Errorf("get in-progress area: %w", err)
	}

	return areas, currentArea, nil
}

func newTurnID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}
