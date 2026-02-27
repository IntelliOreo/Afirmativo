// Package interview handles the mock interview lifecycle.
// This file defines the Question type and the first hardcoded question.
// No infrastructure imports — domain types are infrastructure-free.
package interview

// Question represents a single interview question sent to the client.
type Question struct {
	ID             string
	TextEs         string
	TextEn         string
	FocusArea      string
	QuestionNumber int
	TotalQuestions int
}

// EstimatedTotalQuestions is the approximate number of questions in a full interview.
const EstimatedTotalQuestions = 25

// FirstQuestion returns the opening question for every interview.
func FirstQuestion() *Question {
	return &Question{
		ID:             "q-001",
		TextEs:         "Por favor, diga su nombre completo para el registro.",
		TextEn:         "Please state your full name for the record.",
		FocusArea:      "identity",
		QuestionNumber: 1,
		TotalQuestions: EstimatedTotalQuestions,
	}
}
