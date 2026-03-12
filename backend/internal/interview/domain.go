// Package interview handles the mock interview lifecycle.
// This file defines domain types and the defined focus areas.
// No infrastructure imports — domain types are infrastructure-free.
package interview

import (
	"errors"
	"math"
	"strings"
	"time"
)

// ── Area status constants ──────────────────────────────────────────

// AreaStatus represents the lifecycle state of a question area.
type AreaStatus string

const (
	AreaStatusPending      AreaStatus = "pending"       // Not yet addressed
	AreaStatusPreAddressed AreaStatus = "pre_addressed" // AI flagged as partially covered by a prior answer
	AreaStatusInProgress   AreaStatus = "in_progress"   // Currently being assessed
	AreaStatusComplete     AreaStatus = "complete"      // Sufficient — criterion met
	AreaStatusInsufficient AreaStatus = "insufficient"  // Assessed but not met (follow-up limit or time exhausted)
	AreaStatusNotAssessed  AreaStatus = "not_assessed"  // Interview ended before reaching this criterion
)

// ValidAreaStatuses is the set of all valid status values.
var ValidAreaStatuses = map[AreaStatus]bool{
	AreaStatusPending:      true,
	AreaStatusPreAddressed: true,
	AreaStatusInProgress:   true,
	AreaStatusComplete:     true,
	AreaStatusInsufficient: true,
	AreaStatusNotAssessed:  true,
}

func isAreaUnresolved(status AreaStatus) bool {
	return status == AreaStatusPending ||
		status == AreaStatusPreAddressed ||
		status == AreaStatusInProgress
}

// ── Interview flow constants ──────────────────────────────────────────

type FlowStep string

const (
	FlowStepDisclaimer FlowStep = "disclaimer"
	FlowStepReadiness  FlowStep = "readiness"
	FlowStepCriterion  FlowStep = "criterion"
	FlowStepDone       FlowStep = "done"
)

type QuestionKind string

const (
	QuestionKindDisclaimer QuestionKind = "disclaimer"
	QuestionKindReadiness  QuestionKind = "readiness"
	QuestionKindCriterion  QuestionKind = "criterion"
)

var (
	ErrTurnConflict        = errors.New("turn conflict")
	ErrInvalidFlow         = errors.New("invalid flow state")
	ErrAsyncJobNotFound    = errors.New("async answer job not found")
	ErrIdempotencyConflict = errors.New("idempotency key reused with different payload")
	ErrAIRetryExhausted    = errors.New("ai retry exhausted")
)

type AsyncAnswerJobStatus string

const (
	AsyncAnswerJobQueued    AsyncAnswerJobStatus = "queued"
	AsyncAnswerJobRunning   AsyncAnswerJobStatus = "running"
	AsyncAnswerJobSucceeded AsyncAnswerJobStatus = "succeeded"
	AsyncAnswerJobFailed    AsyncAnswerJobStatus = "failed"
	AsyncAnswerJobConflict  AsyncAnswerJobStatus = "conflict"
	AsyncAnswerJobCanceled  AsyncAnswerJobStatus = "canceled"
)

type CriterionTurnAction string

const (
	CriterionTurnActionStay CriterionTurnAction = "stay"
	CriterionTurnActionNext CriterionTurnAction = "next"
)

type CriterionStatus string

const (
	CriterionStatusSufficient   CriterionStatus = "sufficient"
	CriterionStatusPartial      CriterionStatus = "partially_sufficient"
	CriterionStatusInsufficient CriterionStatus = "insufficient"
)

type CriterionRecommendation string

const (
	CriterionRecMoveOn   CriterionRecommendation = "move_on"
	CriterionRecFollowUp CriterionRecommendation = "follow_up"
)

// CriterionTurnDecision captures policy decisions for one criterion answer.
// The DB adapter applies this decision transactionally.
type CriterionTurnDecision struct {
	Action        CriterionTurnAction
	MarkCurrentAs AreaStatus // complete, insufficient, or empty for no status change
}

// DecideCriterionTurn determines whether to stay on the current criterion
// or move to the next one, based on evaluation outcome and per-area question budget.
func DecideCriterionTurn(current CurrentCriterion, questionsCount, maxQuestionsPerArea int) CriterionTurnDecision {
	if CriterionStatus(strings.TrimSpace(string(current.Status))) == CriterionStatusSufficient {
		return CriterionTurnDecision{
			Action:        CriterionTurnActionNext,
			MarkCurrentAs: AreaStatusComplete,
		}
	}

	if CriterionRecommendation(strings.TrimSpace(string(current.Recommendation))) == CriterionRecMoveOn ||
		(maxQuestionsPerArea > 0 && questionsCount >= maxQuestionsPerArea) {
		return CriterionTurnDecision{
			Action:        CriterionTurnActionNext,
			MarkCurrentAs: AreaStatusInsufficient,
		}
	}

	return CriterionTurnDecision{
		Action:        CriterionTurnActionStay,
		MarkCurrentAs: "",
	}
}

// SelectNextPendingArea returns the first pending/pre_addressed area in the
// configured interview order. Empty string means no remaining area.
func SelectNextPendingArea(orderedAreaSlugs []string, statusByArea map[string]AreaStatus) string {
	for _, slug := range orderedAreaSlugs {
		status, ok := statusByArea[slug]
		if !ok {
			continue
		}
		if status == AreaStatusPending || status == AreaStatusPreAddressed {
			return slug
		}
	}
	return ""
}

func DetermineNextAreaAfterCriterionTurn(
	areas []QuestionArea,
	currentArea string,
	decision CriterionTurnDecision,
	preAddressed []PreAddressedArea,
	orderedAreaSlugs []string,
) string {
	if decision.Action != CriterionTurnActionNext {
		return currentArea
	}

	statusByArea := make(map[string]AreaStatus, len(areas))
	for _, area := range areas {
		statusByArea[area.Area] = area.Status
	}

	if decision.MarkCurrentAs != "" {
		statusByArea[currentArea] = decision.MarkCurrentAs
	}

	for _, flag := range preAddressed {
		if strings.TrimSpace(flag.Slug) == "" {
			continue
		}
		if statusByArea[flag.Slug] == AreaStatusPending {
			statusByArea[flag.Slug] = AreaStatusPreAddressed
		}
	}

	return SelectNextPendingArea(orderedAreaSlugs, statusByArea)
}

// ── Domain types ───────────────────────────────────────────────────

// Question represents a single interview question sent to the client.
type Question struct {
	TextEs         string
	TextEn         string
	Area           string
	Kind           QuestionKind
	TurnID         string
	QuestionNumber int
	TotalQuestions int
}

type IssuedQuestion struct {
	Question         Question
	IssuedAt         time.Time
	AnswerDeadlineAt time.Time
}

func NewIssuedQuestion(question *Question, now time.Time, answerTimeLimitSeconds int) *IssuedQuestion {
	if question == nil {
		return nil
	}
	issuedAt := now.UTC()
	return &IssuedQuestion{
		Question:         *question,
		IssuedAt:         issuedAt,
		AnswerDeadlineAt: issuedAt.Add(time.Duration(answerTimeLimitSeconds) * time.Second),
	}
}

func (q *IssuedQuestion) SubmitWindowRemaining(now time.Time) int {
	if q == nil {
		return 0
	}
	remaining := int(math.Ceil(q.AnswerDeadlineAt.Sub(now.UTC()).Seconds()))
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (q *IssuedQuestion) TextForLanguage(preferredLanguage string) string {
	if q == nil {
		return ""
	}
	if strings.EqualFold(strings.TrimSpace(preferredLanguage), "en") {
		if q.Question.TextEn != "" {
			return q.Question.TextEn
		}
		return q.Question.TextEs
	}
	if q.Question.TextEs != "" {
		return q.Question.TextEs
	}
	return q.Question.TextEn
}

// FlowState represents the interview flow pointer persisted on sessions.
type FlowState struct {
	Step           FlowStep
	ExpectedTurnID string
	QuestionNumber int
	ActiveQuestion *IssuedQuestion
}

// QuestionArea represents a focus area in the interview.
type QuestionArea struct {
	ID                   string
	SessionCode          string
	Area                 string
	Status               AreaStatus
	QuestionsCount       int
	PreAddressedEvidence string
}

// ── Area constants ───────────────────────────────────────────────

// EstimatedTotalQuestions is the approximate number of questions in a full interview.
const EstimatedTotalQuestions = 25

// MaxQuestionsPerArea is the maximum questions (initial + follow-ups) per criterion
// before the backend marks it complete or insufficient and moves on.
const MaxQuestionsPerArea = 6

// ── AI response types ───────────────────────────────────────────────
// These map to the json_schema enforced via output_config in the Claude API call.

// AIResponse is the top-level structure the AI returns.
// On the first turn (opening question), Evaluation will be nil.
type AIResponse struct {
	Evaluation   *Evaluation `json:"evaluation"`
	NextQuestion string      `json:"next_question"`
}

// Evaluation contains the AI's assessment of the candidate's last answer.
type Evaluation struct {
	CurrentCriterion       CurrentCriterion `json:"current_criterion"`
	OtherCriteriaAddressed []OtherCriterion `json:"other_criteria_addressed"`
}

// CurrentCriterion is the AI's judgment on the criterion the backend asked it to evaluate.
type CurrentCriterion struct {
	ID              int                     `json:"id"`
	Status          CriterionStatus         `json:"status"`
	EvidenceSummary string                  `json:"evidence_summary"`
	Recommendation  CriterionRecommendation `json:"recommendation"`
}

// OtherCriterion is flagged when the candidate's answer touched on a different criterion.
type OtherCriterion struct {
	ID              int    `json:"id"`
	Name            string `json:"name"`
	EvidenceSummary string `json:"evidence_summary"`
	Confidence      string `json:"confidence"`
}

// ── Claude API envelope types ───────────────────────────────────────

// ClaudeAPIResponse is the raw HTTP response envelope from the Claude API.
type ClaudeAPIResponse struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"`
	Role         string         `json:"role"`
	Content      []ContentBlock `json:"content"`
	Model        string         `json:"model"`
	StopReason   string         `json:"stop_reason"`
	StopSequence *string        `json:"stop_sequence"`
	Usage        APIUsage       `json:"usage"`
}

// ContentBlock represents one block in the Claude API response content array.
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// APIUsage tracks token consumption.
type APIUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

// ── Turn context types (built by service, consumed by AI client) ────

// AITurnContext holds everything the AI client needs to build an API request.
type AITurnContext struct {
	PreferredLanguage   string
	CurrentAreaSlug     string
	CurrentAreaID       int
	CurrentAreaIndex    int // position in the ordered area list (0-based)
	IsOpeningTurn       bool
	CurrentQuestionText string
	LatestAnswerText    string
	CurrentAreaLabel    string
	Description         string
	SufficiencyReqs     string
	AreaStatus          AreaStatus
	IsPreAddressed      bool
	FollowUpsRemaining  int
	TotalBudgetS        int // total interview budget in seconds (for midpoint calc)
	TimeRemainingS      int
	QuestionsRemaining  int
	CriteriaRemaining   int
	CriteriaCoverage    []CriteriaCoverage
	HistoryTurns        []HistoryTurn
}

// CriteriaCoverage represents one criterion's status for the AI prompt.
type CriteriaCoverage struct {
	ID     int        `json:"id"`
	Name   string     `json:"name"`
	Status AreaStatus `json:"status"`
}

// HistoryTurn represents one completed interview turn in provider-neutral form.
type HistoryTurn struct {
	QuestionText string
	AnswerText   string
}

// ── Answer domain type ──────────────────────────────────────────────

// Answer represents a persisted answer row from the answers table.
type Answer struct {
	ID               string
	SessionCode      string
	Area             string
	QuestionText     string
	TranscriptEs     string
	TranscriptEn     string
	AIEvaluationJSON string
	Sufficiency      string
}

// AnswerJob is the durable async submit record used by polling.
type AnswerJob struct {
	ID              string
	SessionCode     string
	ClientRequestID string
	TurnID          string
	QuestionText    string
	AnswerText      string
	Status          AsyncAnswerJobStatus
	ResultPayload   []byte
	ErrorCode       string
	ErrorMessage    string
	Attempts        int
	StartedAt       *time.Time
	CompletedAt     *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// ── Static questions ───────────────────────────────────────────────

const (
	defaultReadinessQuestionEs = "¿Cómo se siente hoy? ¿Está física y mentalmente preparado/a para continuar con esta entrevista?"
	defaultReadinessQuestionEn = "How are you feeling today? Are you physically and mentally ready to proceed with this interview?"
	defaultOpeningDisclaimerEs = "Aviso importante: esta entrevista simulada es solo para preparacion y no constituye asesoramiento legal. Al continuar, usted confirma que leyo y acepta estos terminos."
	defaultOpeningDisclaimerEn = "Important disclaimer: this mock interview is for preparation only and does not constitute legal advice. By continuing, you confirm that you read and accept these terms."
)

// OpeningDisclaimerQuestion returns the opening disclaimer shown on interview start/resume.
func OpeningDisclaimerQuestion(firstAreaSlug, textEs, textEn string, questionNumber int, turnID string) *Question {
	return &Question{
		TextEs:         firstNonEmpty(textEs, defaultOpeningDisclaimerEs),
		TextEn:         firstNonEmpty(textEn, defaultOpeningDisclaimerEn),
		Area:           firstAreaSlug,
		Kind:           QuestionKindDisclaimer,
		TurnID:         turnID,
		QuestionNumber: questionNumber,
		TotalQuestions: EstimatedTotalQuestions,
	}
}

// ReadinessQuestion returns the non-criteria readiness question shown after disclaimer confirmation.
func ReadinessQuestion(firstAreaSlug, textEs, textEn string, questionNumber int, turnID string) *Question {
	return &Question{
		TextEs:         firstNonEmpty(textEs, defaultReadinessQuestionEs),
		TextEn:         firstNonEmpty(textEn, defaultReadinessQuestionEn),
		Area:           firstAreaSlug,
		Kind:           QuestionKindReadiness,
		TurnID:         turnID,
		QuestionNumber: questionNumber,
		TotalQuestions: EstimatedTotalQuestions,
	}
}

// ResumeQuestion returns a confirmation question shown when resuming an interview.
func ResumeQuestion(firstAreaSlug string) *Question {
	return &Question{
		TextEs:         "Bienvenido/a de nuevo. Su entrevista fue iniciada anteriormente. ¿Está listo/a para continuar donde lo dejó?",
		TextEn:         "Welcome back. Your interview was started previously. Are you ready to continue where you left off?",
		Area:           firstAreaSlug,
		Kind:           QuestionKindReadiness,
		QuestionNumber: 1,
		TotalQuestions: EstimatedTotalQuestions,
	}
}

func firstNonEmpty(v, fallback string) string {
	if strings.TrimSpace(v) != "" {
		return v
	}
	return fallback
}
