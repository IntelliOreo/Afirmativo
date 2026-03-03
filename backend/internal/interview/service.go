// Service layer for interview operations.
// StartInterview: sets session to interviewing, creates areas, returns first question.
// SubmitAnswer: persists answer, evaluates via AI, manages area transitions.
package interview

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/afirmativo/backend/internal/config"
	"github.com/afirmativo/backend/internal/session"
)

const dbTimeout = 5 * time.Second

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

// AIClient calls the AI API to evaluate answers and generate next questions.
type AIClient interface {
	CallAI(ctx context.Context, turnCtx *AITurnContext) (*AIResponse, error)
}

// Service contains interview business logic.
type Service struct {
	sessionStarter   SessionStarter
	sessionGetter    SessionGetter
	sessionCompleter SessionCompleter
	store            Store
	aiClient         AIClient
	areaConfigs      []config.AreaConfig
	openingTextEn    string
	openingTextEs    string
	readinessTextEn  string
	readinessTextEs  string
}

// NewService creates a Service with the given dependencies.
func NewService(
	ss SessionStarter,
	sg SessionGetter,
	sc SessionCompleter,
	store Store,
	ai AIClient,
	areaConfigs []config.AreaConfig,
	openingTextEn, openingTextEs, readinessTextEn, readinessTextEs string,
) *Service {
	return &Service{
		sessionStarter:   ss,
		sessionGetter:    sg,
		sessionCompleter: sc,
		store:            store,
		aiClient:         ai,
		areaConfigs:      areaConfigs,
		openingTextEn:    openingTextEn,
		openingTextEs:    openingTextEs,
		readinessTextEn:  readinessTextEn,
		readinessTextEs:  readinessTextEs,
	}
}

// StartResult holds the output of a successful interview start.
type StartResult struct {
	Question        *Question
	TimerRemainingS int
	Area            string
	Resuming        bool
	Language        string
}

// StartInterview transitions the session to interviewing,
// creates all question area rows, and returns the opening question.
func (s *Service) StartInterview(ctx context.Context, sessionCode, preferredLanguage string) (*StartResult, error) {
	dbCtx, dbCancel := context.WithTimeout(ctx, dbTimeout)
	defer dbCancel()

	sess, err := s.sessionStarter.StartSession(dbCtx, sessionCode, preferredLanguage)
	if err != nil {
		return nil, err
	}
	effectiveLanguage := normalizePreferredLanguage(sess.PreferredLanguage)

	remaining := sess.InterviewBudgetSeconds - sess.InterviewLapsedSeconds

	answersCount, err := s.store.GetAnswerCount(dbCtx, sessionCode)
	if err != nil {
		return nil, fmt.Errorf("get answer count: %w", err)
	}

	// Resume only after there is actual prior progress. This avoids showing
	// "Welcome back" on first entry if the start endpoint is called twice.
	resuming := answersCount > 0

	// Pre-create all 8 question area rows (idempotent — ON CONFLICT DO NOTHING).
	for _, area := range s.areaConfigs {
		if _, err := s.store.CreateQuestionArea(dbCtx, sessionCode, area.Slug); err != nil {
			return nil, fmt.Errorf("create question area %s: %w", area.Slug, err)
		}
	}

	// Set the first area to in_progress (no-op if already in_progress from a prior start).
	firstArea := s.areaConfigs[0].Slug
	if err := s.store.SetAreaInProgress(dbCtx, sessionCode, firstArea); err != nil {
		return nil, fmt.Errorf("set first area in_progress: %w", err)
	}

	turnID, err := newTurnID()
	if err != nil {
		return nil, fmt.Errorf("new turn id: %w", err)
	}
	flowState, err := s.store.PrepareDisclaimerStep(dbCtx, sessionCode, turnID)
	if err != nil {
		return nil, fmt.Errorf("prepare disclaimer step: %w", err)
	}

	q := OpeningDisclaimerQuestion(
		firstArea,
		s.openingTextEs,
		s.openingTextEn,
		flowState.QuestionNumber,
		turnID,
	)

	return &StartResult{
		Question:        q,
		TimerRemainingS: remaining,
		Area:            firstArea,
		Resuming:        resuming,
		Language:        effectiveLanguage,
	}, nil
}

// AnswerResult holds the output of a submitted answer.
type AnswerResult struct {
	Done            bool
	NextQuestion    *Question
	TimerRemainingS int
}

// SubmitAnswer processes one answer according to the explicit flow step.
func (s *Service) SubmitAnswer(ctx context.Context, sessionCode, answerText, questionText, turnID string) (*AnswerResult, error) {
	dbCtx, dbCancel := context.WithTimeout(ctx, dbTimeout)
	defer dbCancel()

	sess, err := s.sessionGetter.GetSessionByCode(dbCtx, sessionCode)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	flowState, err := s.store.GetFlowState(dbCtx, sessionCode)
	if err != nil {
		return nil, fmt.Errorf("get flow state: %w", err)
	}

	areas, currentArea, err := s.refreshAreaState(dbCtx, sessionCode)
	if err != nil {
		return nil, fmt.Errorf("refresh area state: %w", err)
	}

	if flowState.Step == FlowStepDone {
		s.finishSession(ctx, sessionCode)
		return &AnswerResult{Done: true, TimerRemainingS: 0}, nil
	}
	if strings.TrimSpace(turnID) == "" || turnID != flowState.ExpectedTurnID {
		return nil, ErrTurnConflict
	}

	timeRemainingS := s.calcTimeRemaining(sess)
	if timeRemainingS <= 0 {
		s.markRemainingNotAssessed(ctx, sessionCode, areas)
		if err := s.store.MarkFlowDone(ctx, sessionCode); err != nil {
			slog.Warn("failed to mark flow done on timeout", "session", sessionCode, "error", err)
		}
		s.finishSession(ctx, sessionCode)
		return &AnswerResult{Done: true, TimerRemainingS: 0}, nil
	}

	preferredLanguage := normalizePreferredLanguage(sess.PreferredLanguage)

	switch flowState.Step {
	case FlowStepDisclaimer:
		if currentArea == nil {
			s.finishSession(ctx, sessionCode)
			return &AnswerResult{Done: true, TimerRemainingS: 0}, nil
		}

		nextTurnID, err := newTurnID()
		if err != nil {
			return nil, fmt.Errorf("new turn id: %w", err)
		}
		nextFlow, err := s.store.AdvanceNonCriterionStep(dbCtx, AdvanceNonCriterionStepParams{
			SessionCode:    sessionCode,
			ExpectedTurnID: turnID,
			CurrentStep:    FlowStepDisclaimer,
			NextStep:       FlowStepReadiness,
			NextTurnID:     nextTurnID,
			EventType:      "disclaimer_ack",
			AnswerText:     answerText,
		})
		if err != nil {
			return nil, fmt.Errorf("advance disclaimer step: %w", err)
		}

		return &AnswerResult{
			Done: false,
			NextQuestion: ReadinessQuestion(
				currentArea.Area,
				s.readinessTextEs,
				s.readinessTextEn,
				nextFlow.QuestionNumber,
				nextTurnID,
			),
			TimerRemainingS: timeRemainingS,
		}, nil

	case FlowStepReadiness:
		if currentArea == nil {
			s.finishSession(ctx, sessionCode)
			return &AnswerResult{Done: true, TimerRemainingS: 0}, nil
		}

		nextTurnID, err := newTurnID()
		if err != nil {
			return nil, fmt.Errorf("new turn id: %w", err)
		}
		nextFlow, err := s.store.AdvanceNonCriterionStep(dbCtx, AdvanceNonCriterionStepParams{
			SessionCode:    sessionCode,
			ExpectedTurnID: turnID,
			CurrentStep:    FlowStepReadiness,
			NextStep:       FlowStepCriterion,
			NextTurnID:     nextTurnID,
			EventType:      "readiness_ack",
			AnswerText:     answerText,
		})
		if err != nil {
			return nil, fmt.Errorf("advance readiness step: %w", err)
		}

		nextQuestion := s.fallbackQuestionForArea(currentArea.Area)
		return &AnswerResult{
			Done: false,
			NextQuestion: &Question{
				TextEs:         nextQuestion,
				TextEn:         nextQuestion,
				Area:           currentArea.Area,
				Kind:           QuestionKindCriterion,
				TurnID:         nextTurnID,
				QuestionNumber: nextFlow.QuestionNumber,
				TotalQuestions: EstimatedTotalQuestions,
			},
			TimerRemainingS: timeRemainingS,
		}, nil

	case FlowStepCriterion:
		if currentArea == nil {
			if err := s.store.MarkFlowDone(ctx, sessionCode); err != nil {
				slog.Warn("failed to mark flow done with no in-progress area", "session", sessionCode, "error", err)
			}
			s.finishSession(ctx, sessionCode)
			return &AnswerResult{Done: true, TimerRemainingS: 0}, nil
		}

		answers, err := s.store.GetAnswersBySession(dbCtx, sessionCode)
		if err != nil {
			return nil, fmt.Errorf("get answers: %w", err)
		}

		areaCfg, areaIndex := s.findAreaConfig(currentArea.Area)
		criteriaCoverage := s.buildCriteriaCoverage(areas)
		transcript := s.buildTranscript(answers)
		criteriaRemaining := s.countCriteriaRemaining(areas)

		turnCtx := &AITurnContext{
			PreferredLanguage:  preferredLanguage,
			CurrentAreaSlug:    currentArea.Area,
			CurrentAreaID:      areaCfg.ID,
			CurrentAreaIndex:   areaIndex,
			CurrentAreaLabel:   areaCfg.Label,
			Description:        areaCfg.Description,
			SufficiencyReqs:    areaCfg.SufficiencyRequirements,
			AreaStatus:         string(currentArea.Status),
			IsPreAddressed:     currentArea.Status == AreaStatusPreAddressed,
			FollowUpsRemaining: MaxQuestionsPerArea - currentArea.QuestionsCount,
			TotalBudgetS:       sess.InterviewBudgetSeconds,
			TimeRemainingS:     timeRemainingS,
			QuestionsRemaining: EstimatedTotalQuestions - len(answers),
			CriteriaRemaining:  criteriaRemaining,
			CriteriaCoverage:   criteriaCoverage,
			Transcript:         transcript,
		}

		slog.Debug("calling AI for criterion turn", "session", sessionCode, "area", currentArea.Area)
		aiResult, err := s.aiClient.CallAI(ctx, turnCtx)
		if err != nil {
			slog.Warn("AI API error, using fallback evaluation", "error", err, "area", currentArea.Area)
			aiResult = &AIResponse{
				Evaluation:   s.fallbackEvaluation(areaCfg.ID),
				NextQuestion: s.fallbackQuestionForArea(currentArea.Area),
			}
		}

		if aiResult.Evaluation == nil || aiResult.Evaluation.CurrentCriterion.ID != areaCfg.ID {
			if aiResult.Evaluation != nil {
				slog.Warn("AI evaluation criterion mismatch, replacing with fallback",
					"session", sessionCode,
					"current_area", currentArea.Area,
					"expected_criterion_id", areaCfg.ID,
					"returned_criterion_id", aiResult.Evaluation.CurrentCriterion.ID,
				)
			}
			aiResult.Evaluation = s.fallbackEvaluation(areaCfg.ID)
		}

		nextTurnID, err := newTurnID()
		if err != nil {
			return nil, fmt.Errorf("new turn id: %w", err)
		}

		preAddressed := s.extractPreAddressed(aiResult.Evaluation.OtherCriteriaAddressed)
		result, err := s.store.ProcessCriterionTurn(dbCtx, ProcessCriterionTurnParams{
			SessionCode:         sessionCode,
			ExpectedTurnID:      turnID,
			CurrentArea:         currentArea.Area,
			QuestionText:        questionText,
			AnswerText:          answerText,
			PreferredLanguage:   preferredLanguage,
			Evaluation:          aiResult.Evaluation,
			PreAddressed:        preAddressed,
			OrderedAreaSlugs:    s.orderedAreaSlugs(),
			MaxQuestionsPerArea: MaxQuestionsPerArea,
			NextTurnID:          nextTurnID,
		})
		if err != nil {
			if errors.Is(err, ErrTurnConflict) {
				return nil, ErrTurnConflict
			}
			return nil, fmt.Errorf("process criterion turn: %w", err)
		}

		areas, _, err = s.refreshAreaState(dbCtx, sessionCode)
		if err != nil {
			return nil, fmt.Errorf("refresh areas after criterion: %w", err)
		}

		timeRemainingS = s.calcTimeRemaining(sess)
		if timeRemainingS <= 0 {
			s.markRemainingNotAssessed(ctx, sessionCode, areas)
			if err := s.store.MarkFlowDone(ctx, sessionCode); err != nil {
				slog.Warn("failed to mark flow done on timeout after criterion", "session", sessionCode, "error", err)
			}
			s.finishSession(ctx, sessionCode)
			return &AnswerResult{Done: true, TimerRemainingS: 0}, nil
		}

		if strings.TrimSpace(result.NextArea) == "" {
			if err := s.store.MarkFlowDone(ctx, sessionCode); err != nil {
				slog.Warn("failed to mark flow done on final criterion", "session", sessionCode, "error", err)
			}
			s.finishSession(ctx, sessionCode)
			return &AnswerResult{Done: true, TimerRemainingS: 0}, nil
		}

		nextQuestion := strings.TrimSpace(aiResult.NextQuestion)
		if result.Action == "next" {
			nextQuestion = s.fallbackQuestionForArea(result.NextArea)
		}
		if nextQuestion == "" {
			nextQuestion = s.fallbackQuestionForArea(result.NextArea)
		}

		return &AnswerResult{
			Done: false,
			NextQuestion: &Question{
				TextEs:         nextQuestion,
				TextEn:         nextQuestion,
				Area:           result.NextArea,
				Kind:           QuestionKindCriterion,
				TurnID:         nextTurnID,
				QuestionNumber: result.QuestionNumber,
				TotalQuestions: EstimatedTotalQuestions,
			},
			TimerRemainingS: timeRemainingS,
		}, nil
	default:
		return nil, ErrInvalidFlow
	}
}

// finishSession marks the session as completed. Logs on error but does not
// propagate — the interview result has already been determined.
func (s *Service) finishSession(ctx context.Context, sessionCode string) {
	if err := s.sessionCompleter.CompleteSession(ctx, sessionCode); err != nil {
		slog.Error("failed to complete session", "session", sessionCode, "error", err)
	}
}

func (s *Service) orderedAreaSlugs() []string {
	slugs := make([]string, 0, len(s.areaConfigs))
	for _, cfg := range s.areaConfigs {
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
			Status:          "partially_sufficient",
			EvidenceSummary: "Fallback evaluation due to model parsing or provider error.",
			Recommendation:  "follow_up",
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

// ── Helper methods ──────────────────────────────────────────────────

func (s *Service) calcTimeRemaining(sess *session.Session) int {
	if sess.CurrentInterviewStartedAt == nil {
		return sess.InterviewBudgetSeconds - sess.InterviewLapsedSeconds
	}
	elapsed := int(time.Since(*sess.CurrentInterviewStartedAt).Seconds())
	remaining := sess.InterviewBudgetSeconds - sess.InterviewLapsedSeconds - elapsed
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (s *Service) findAreaConfig(slug string) (config.AreaConfig, int) {
	for i, ac := range s.areaConfigs {
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
			Status: string(a.Status),
		})
	}
	return coverage
}

func (s *Service) buildTranscript(answers []Answer) []TranscriptEntry {
	transcript := make([]TranscriptEntry, 0, len(answers))
	for i, a := range answers {
		answerText := a.TranscriptEs
		if a.TranscriptEn != "" {
			answerText = a.TranscriptEn
		}
		transcript = append(transcript, TranscriptEntry{
			QuestionNumber: i + 1,
			Criterion:      a.Area,
			Question:       a.QuestionText,
			Answer:         answerText,
		})
	}
	return transcript
}

func (s *Service) countCriteriaRemaining(areas []QuestionArea) int {
	count := 0
	for _, a := range areas {
		if a.Status != AreaStatusComplete && a.Status != AreaStatusInsufficient && a.Status != AreaStatusNotAssessed {
			count++
		}
	}
	return count
}

// matchAreaSlug tries to find a matching area slug from the AI's cross-criteria name.
// Uses case-insensitive matching against both slugs and labels.
func (s *Service) matchAreaSlug(name string) string {
	lower := strings.ToLower(name)
	for _, ac := range s.areaConfigs {
		if strings.ToLower(ac.Slug) == lower || strings.ToLower(ac.Label) == lower {
			return ac.Slug
		}
	}
	return ""
}

func (s *Service) markRemainingNotAssessed(ctx context.Context, sessionCode string, areas []QuestionArea) {
	dbCtx, dbCancel := context.WithTimeout(ctx, dbTimeout)
	defer dbCancel()
	for _, a := range areas {
		if a.Status == AreaStatusPending || a.Status == AreaStatusPreAddressed || a.Status == AreaStatusInProgress {
			if err := s.store.MarkAreaNotAssessed(dbCtx, sessionCode, a.Area); err != nil {
				slog.Warn("failed to mark not_assessed", "area", a.Area, "error", err)
			}
		}
	}
}

func (s *Service) refreshAreaState(ctx context.Context, sessionCode string) ([]QuestionArea, *QuestionArea, error) {
	dbCtx, dbCancel := context.WithTimeout(ctx, dbTimeout)
	defer dbCancel()

	areas, err := s.store.GetAreasBySession(dbCtx, sessionCode)
	if err != nil {
		return nil, nil, fmt.Errorf("get areas by session: %w", err)
	}

	currentArea, err := s.store.GetInProgressArea(dbCtx, sessionCode)
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

func normalizePreferredLanguage(language string) string {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "en":
		return "en"
	default:
		return "es"
	}
}
