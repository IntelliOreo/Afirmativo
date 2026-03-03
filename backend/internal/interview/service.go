// Service layer for interview operations.
// StartInterview: sets session to interviewing, creates areas, returns first question.
// SubmitAnswer: persists answer, evaluates via AI, manages area transitions.
package interview

import (
	"context"
	"encoding/json"
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

	var q *Question
	if resuming {
		q = ResumeQuestion(firstArea)
	} else {
		q = OpeningDisclaimerQuestion(firstArea, s.openingTextEs, s.openingTextEn)
	}

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

// SubmitAnswer processes a user's answer: persists it, evaluates via AI,
// manages area transitions, and returns the next question.
func (s *Service) SubmitAnswer(ctx context.Context, sessionCode, answerText, questionText string, questionNumber int) (*AnswerResult, error) {
	dbCtx, dbCancel := context.WithTimeout(ctx, dbTimeout)
	defer dbCancel()

	// 1. Load current state from DB.
	sess, err := s.sessionGetter.GetSessionByCode(dbCtx, sessionCode)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}

	areas, err := s.store.GetAreasBySession(dbCtx, sessionCode)
	if err != nil {
		return nil, fmt.Errorf("get areas: %w", err)
	}

	answers, err := s.store.GetAnswersBySession(dbCtx, sessionCode)
	if err != nil {
		return nil, fmt.Errorf("get answers: %w", err)
	}

	currentArea, err := s.store.GetInProgressArea(dbCtx, sessionCode)
	if err != nil {
		return nil, fmt.Errorf("get in-progress area: %w", err)
	}
	if currentArea == nil {
		s.finishSession(ctx, sessionCode)
		return &AnswerResult{Done: true}, nil
	}

	// 2. Calculate timer.
	timeRemainingS := s.calcTimeRemaining(sess)
	timeExpired := timeRemainingS <= 0
	preferredLanguage := normalizePreferredLanguage(sess.PreferredLanguage)

	// Opening confirmation and readiness are non-criteria turns on first entry.
	// Do not evaluate, persist, or consume criterion quota during these steps.
	if questionNumber == 1 {
		if timeExpired {
			s.markRemainingNotAssessed(ctx, sessionCode, areas)
			s.finishSession(ctx, sessionCode)
			return &AnswerResult{Done: true, TimerRemainingS: 0}, nil
		}

		if len(answers) == 0 {
			return &AnswerResult{
				Done: false,
				NextQuestion: ReadinessQuestion(
					currentArea.Area,
					s.readinessTextEs,
					s.readinessTextEn,
					2,
				),
				TimerRemainingS: timeRemainingS,
			}, nil
		}

		areaCfg, _ := s.findAreaConfig(currentArea.Area)
		nextQuestion := strings.TrimSpace(areaCfg.FallbackQuestion)
		if nextQuestion == "" {
			nextQuestion = fmt.Sprintf("Please tell me about %s.", areaCfg.Label)
		}

		return &AnswerResult{
			Done: false,
			NextQuestion: &Question{
				TextEs:         nextQuestion,
				TextEn:         nextQuestion,
				Area:           currentArea.Area,
				QuestionNumber: 2,
				TotalQuestions: EstimatedTotalQuestions,
			},
			TimerRemainingS: timeRemainingS,
		}, nil
	}

	if len(answers) == 0 && questionNumber == 2 && timeExpired {
		s.markRemainingNotAssessed(ctx, sessionCode, areas)
		s.finishSession(ctx, sessionCode)
		return &AnswerResult{Done: true, TimerRemainingS: 0}, nil
	}

	// 3. Build AI turn context.
	areaCfg, areaIndex := s.findAreaConfig(currentArea.Area)
	criteriaCoverage := s.buildCriteriaCoverage(areas)
	transcript := s.buildTranscript(answers)
	criteriaRemaining := s.countCriteriaRemaining(areas)

	slog.Debug("building AI turn context",
		"session", sessionCode,
		"area", currentArea.Area,
		"area_index", areaIndex,
		"area_status", currentArea.Status,
		"questions_count", currentArea.QuestionsCount,
		"time_remaining_s", timeRemainingS,
		"total_budget_s", sess.InterviewBudgetSeconds,
		"criteria_remaining", criteriaRemaining,
		"transcript_len", len(transcript),
	)

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

	// 4. Call AI (with fallback on error).
	slog.Debug("calling AI for turn", "session", sessionCode, "area", currentArea.Area)
	aiResult, err := s.aiClient.CallAI(ctx, turnCtx)
	if err != nil {
		slog.Warn("AI API error, using fallback", "error", err, "area", currentArea.Area)
		aiResult = &AIResponse{
			Evaluation:   nil,
			NextQuestion: areaCfg.FallbackQuestion,
		}
	} else {
		slog.Debug("AI call succeeded", "session", sessionCode, "next_question", aiResult.NextQuestion)
	}

	// Defensive guard: if model returns evaluation for a different criterion id,
	// ignore it so area progression stays aligned with backend state.
	if aiResult.Evaluation != nil && aiResult.Evaluation.CurrentCriterion.ID != areaCfg.ID {
		slog.Warn("ignoring AI evaluation with mismatched criterion id",
			"session", sessionCode,
			"current_area", currentArea.Area,
			"expected_criterion_id", areaCfg.ID,
			"returned_criterion_id", aiResult.Evaluation.CurrentCriterion.ID,
		)
		aiResult.Evaluation = nil
	}

	// First-entry readiness answer (question #2) should not be persisted or scored.
	// Use AI only to generate the first criterion question.
	if len(answers) == 0 && questionNumber == 2 {
		nextQuestion := strings.TrimSpace(aiResult.NextQuestion)
		if nextQuestion == "" {
			nextQuestion = strings.TrimSpace(areaCfg.FallbackQuestion)
		}
		if nextQuestion == "" {
			nextQuestion = fmt.Sprintf("Please tell me about %s.", areaCfg.Label)
		}
		return &AnswerResult{
			Done: false,
			NextQuestion: &Question{
				TextEs:         nextQuestion,
				TextEn:         nextQuestion,
				Area:           currentArea.Area,
				QuestionNumber: 3,
				TotalQuestions: EstimatedTotalQuestions,
			},
			TimerRemainingS: timeRemainingS,
		}, nil
	}

	// 5. Process the turn (persist answer, update areas).
	nextAction, err := s.processTurn(ctx, sessionCode, currentArea, answerText, questionText, questionNumber, preferredLanguage, aiResult)
	if err != nil {
		return nil, fmt.Errorf("process turn: %w", err)
	}

	// Recalculate timer after processing.
	timeRemainingS = s.calcTimeRemaining(sess)
	if !timeExpired {
		timeExpired = timeRemainingS <= 0
	}

	// 6. Determine what to return.
	// If time expired, we already processed the final answer above — now finish up.
	if timeExpired || nextAction == "end" {
		s.markRemainingNotAssessed(ctx, sessionCode, areas)
		s.finishSession(ctx, sessionCode)
		return &AnswerResult{Done: true, TimerRemainingS: 0}, nil
	}

	// Find the area for the next question.
	nextAreaSlug := currentArea.Area
	if nextAction == "next" {
		nextAreaSlug, err = s.advanceToNextArea(ctx, sessionCode, areas)
		if err != nil {
			return nil, fmt.Errorf("advance area: %w", err)
		}
		if nextAreaSlug == "" {
			s.finishSession(ctx, sessionCode)
			return &AnswerResult{Done: true, TimerRemainingS: 0}, nil
		}
	}

	nextNum := questionNumber + 1

	return &AnswerResult{
		Done: false,
		NextQuestion: &Question{
			TextEs:         aiResult.NextQuestion,
			TextEn:         aiResult.NextQuestion,
			Area:           nextAreaSlug,
			QuestionNumber: nextNum,
			TotalQuestions: EstimatedTotalQuestions,
		},
		TimerRemainingS: timeRemainingS,
	}, nil
}

// finishSession marks the session as completed. Logs on error but does not
// propagate — the interview result has already been determined.
func (s *Service) finishSession(ctx context.Context, sessionCode string) {
	if err := s.sessionCompleter.CompleteSession(ctx, sessionCode); err != nil {
		slog.Error("failed to complete session", "session", sessionCode, "error", err)
	}
}

// processTurn handles the evaluation and DB writes for one turn.
// Returns "stay", "next", or "end".
func (s *Service) processTurn(
	ctx context.Context,
	sessionCode string,
	currentArea *QuestionArea,
	answerText, questionText string,
	questionNumber int,
	preferredLanguage string,
	aiResult *AIResponse,
) (string, error) {
	eval := aiResult.Evaluation

	// First turn (opening question) — no evaluation to process.
	if eval == nil {
		return "stay", nil
	}

	dbCtx, dbCancel := context.WithTimeout(ctx, dbTimeout)
	defer dbCancel()

	var transcriptEs, transcriptEn string
	if preferredLanguage == "en" {
		transcriptEn = answerText
	} else {
		transcriptEs = answerText
	}

	// a. Save the answer.
	evalJSON, _ := json.Marshal(eval)
	_, err := s.store.SaveAnswer(dbCtx, SaveAnswerParams{
		SessionCode:  sessionCode,
		Area:         currentArea.Area, // backend decides this, not AI
		QuestionText: questionText,
		TranscriptEs: transcriptEs,
		TranscriptEn: transcriptEn,
		AiEvaluation: evalJSON,
		Sufficiency:  eval.CurrentCriterion.Status,
	})
	if err != nil {
		return "", fmt.Errorf("save answer: %w", err)
	}

	// b. Increment question count for current area.
	if err := s.store.IncrementAreaQuestions(dbCtx, sessionCode, currentArea.Area); err != nil {
		return "", fmt.Errorf("increment questions: %w", err)
	}

	newCount := currentArea.QuestionsCount + 1
	action := "stay"

	// c. Determine action based on evaluation + backend rules.
	if eval.CurrentCriterion.Status == "sufficient" {
		if err := s.store.CompleteArea(dbCtx, sessionCode, currentArea.Area); err != nil {
			return "", fmt.Errorf("complete area: %w", err)
		}
		action = "next"
	} else if newCount >= MaxQuestionsPerArea {
		// Follow-up budget exhausted.
		if err := s.store.MarkAreaInsufficient(dbCtx, sessionCode, currentArea.Area); err != nil {
			return "", fmt.Errorf("mark insufficient (budget): %w", err)
		}
		action = "next"
	} else if eval.CurrentCriterion.Recommendation == "move_on" {
		// AI says move on even with budget remaining.
		if err := s.store.MarkAreaInsufficient(dbCtx, sessionCode, currentArea.Area); err != nil {
			return "", fmt.Errorf("mark insufficient (move_on): %w", err)
		}
		action = "next"
	}

	// d. Process cross-criteria flags.
	for _, other := range eval.OtherCriteriaAddressed {
		slug := s.matchAreaSlug(other.Name)
		if slug == "" {
			slog.Warn("cross-criteria flag: no matching area", "name", other.Name)
			continue
		}
		if err := s.store.MarkAreaPreAddressed(dbCtx, sessionCode, slug, other.EvidenceSummary); err != nil {
			slog.Warn("failed to mark pre_addressed", "area", slug, "error", err)
		}
	}

	slog.Info("turn processed",
		"session", sessionCode,
		"area", currentArea.Area,
		"status", eval.CurrentCriterion.Status,
		"recommendation", eval.CurrentCriterion.Recommendation,
		"action", action,
		"questions_count", newCount,
	)

	return action, nil
}

// advanceToNextArea finds the next pending/pre_addressed area in sequence,
// sets it to in_progress, and returns its slug. Returns "" if no more areas.
func (s *Service) advanceToNextArea(ctx context.Context, sessionCode string, areas []QuestionArea) (string, error) {
	dbCtx, dbCancel := context.WithTimeout(ctx, dbTimeout)
	defer dbCancel()

	// Walk areaConfigs in order to find the next uncovered one.
	areaStatusMap := make(map[string]AreaStatus)
	for _, a := range areas {
		areaStatusMap[a.Area] = a.Status
	}

	for _, ac := range s.areaConfigs {
		status, ok := areaStatusMap[ac.Slug]
		if !ok {
			continue
		}
		if status == AreaStatusPending || status == AreaStatusPreAddressed {
			if err := s.store.SetAreaInProgress(dbCtx, sessionCode, ac.Slug); err != nil {
				return "", fmt.Errorf("set area in_progress: %w", err)
			}
			return ac.Slug, nil
		}
	}

	return "", nil // all areas done
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

func normalizePreferredLanguage(language string) string {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "en":
		return "en"
	default:
		return "es"
	}
}
