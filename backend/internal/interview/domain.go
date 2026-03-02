// Package interview handles the mock interview lifecycle.
// This file defines domain types and the defined focus areas.
// No infrastructure imports — domain types are infrastructure-free.
package interview

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

// ── Domain types ───────────────────────────────────────────────────

// Question represents a single interview question sent to the client.
type Question struct {
	TextEs         string
	TextEn         string
	Area           string
	QuestionNumber int
	TotalQuestions int
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
	ID              int    `json:"id"`
	Status          string `json:"status"`
	EvidenceSummary string `json:"evidence_summary"`
	Recommendation  string `json:"recommendation"`
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
	CurrentAreaSlug    string
	CurrentAreaID      int
	CurrentAreaIndex   int // position in the ordered area list (0-based)
	CurrentAreaLabel   string
	Description        string
	SufficiencyReqs    string
	AreaStatus         string
	IsPreAddressed     bool
	FollowUpsRemaining int
	TotalBudgetS       int // total interview budget in seconds (for midpoint calc)
	TimeRemainingS     int
	QuestionsRemaining int
	CriteriaRemaining  int
	CriteriaCoverage   []CriteriaCoverage
	Transcript         []TranscriptEntry
}

// CriteriaCoverage represents one criterion's status for the AI prompt.
type CriteriaCoverage struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// TranscriptEntry represents one Q&A pair in the interview history.
type TranscriptEntry struct {
	QuestionNumber int    `json:"question_number"`
	Criterion      string `json:"criterion"`
	Question       string `json:"question"`
	Answer         string `json:"answer"`
}

// ── Answer domain type ──────────────────────────────────────────────

// Answer represents a persisted answer row from the answers table.
type Answer struct {
	ID           string
	SessionCode  string
	Area         string
	QuestionText string
	TranscriptEs string
	TranscriptEn string
	AiEvaluation string
	Sufficiency  string
}

// ── Static questions ───────────────────────────────────────────────

// FirstQuestion returns the opening question for every interview.
func FirstQuestion(firstAreaSlug string) *Question {
	return &Question{
		TextEs:         "¿Cómo se siente hoy? ¿Está física y mentalmente preparado/a para continuar con esta entrevista?",
		TextEn:         "How are you feeling today? Are you physically and mentally ready to proceed with this interview?",
		Area:           firstAreaSlug,
		QuestionNumber: 1,
		TotalQuestions: EstimatedTotalQuestions,
	}
}

// ResumeQuestion returns a confirmation question shown when resuming an interview.
func ResumeQuestion(firstAreaSlug string) *Question {
	return &Question{
		TextEs:         "Bienvenido/a de nuevo. Su entrevista fue iniciada anteriormente. ¿Está listo/a para continuar donde lo dejó?",
		TextEn:         "Welcome back. Your interview was started previously. Are you ready to continue where you left off?",
		Area:           firstAreaSlug,
		QuestionNumber: 1,
		TotalQuestions: EstimatedTotalQuestions,
	}
}
