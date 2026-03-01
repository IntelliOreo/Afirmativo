// Package interview handles the mock interview lifecycle.
// This file defines domain types and the defined focus areas.
// No infrastructure imports — domain types are infrastructure-free.
package interview

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
	ID             string
	SessionCode    string
	Area           string
	Status         string // in_progress | sufficient | insufficient | skipped
	QuestionsCount int
}

// EstimatedTotalQuestions is the approximate number of questions in a full interview.
const EstimatedTotalQuestions = 25

// AreaInfo holds the slug and display label for a focus area.
type AreaInfo struct {
	Slug  string
	Label string
}

// DefinedAreas is the ordered list of asylum evaluation criteria.
var DefinedAreas = []AreaInfo{
	{Slug: "persecution", Label: "Persecution or well-founded fear"},
	{Slug: "protected_ground", Label: "Protected ground"},
	{Slug: "nexus", Label: "Nexus"},
	{Slug: "perpetrator", Label: "Perpetrator (a group the gov cannot or will not control)"},
	{Slug: "one_year", Label: "Filed in one year (with exceptions)"},
	{Slug: "resettlement", Label: "Not firmly resettled in a third country"},
	{Slug: "bars", Label: "Not barred (e.g., serious crime)"},
}

// FirstQuestion returns the opening question for every interview.
func FirstQuestion() *Question {
	return &Question{
		TextEs:         "¿Cómo se siente hoy? ¿Está física y mentalmente preparado/a para continuar con esta entrevista?",
		TextEn:         "How are you feeling today? Are you physically and mentally ready to proceed with this interview?",
		Area:           DefinedAreas[0].Slug,
		QuestionNumber: 1,
		TotalQuestions: EstimatedTotalQuestions,
	}
}
