// Package report handles report retrieval and generation.
// This file defines the Report domain type — no infrastructure imports.
package report

import "errors"

var (
	ErrSessionNotFound     = errors.New("report session not found")
	ErrSessionNotCompleted = errors.New("report session not completed")
)

type ReportStatus string

const (
	ReportStatusGenerating ReportStatus = "generating"
	ReportStatusReady      ReportStatus = "ready"
	ReportStatusFailed     ReportStatus = "failed"
)

// Report represents a completed assessment report.
type Report struct {
	SessionCode             string
	Status                  ReportStatus
	ContentEn               string
	ContentEs               string
	AreasOfClarity          []string
	AreasOfClarityEs        []string
	AreasToDevelopFurther   []string
	AreasToDevelopFurtherEs []string
	Recommendation          string
	RecommendationEs        string
	QuestionCount           int
	DurationMinutes         int
}

// AreaSummary is a compact representation of one area's evaluation result,
// used to build the AI report prompt.
type AreaSummary struct {
	Slug            string `json:"slug"`
	Label           string `json:"label"`
	Status          string `json:"status"`
	EvidenceSummary string `json:"evidence_summary"`
	Recommendation  string `json:"recommendation"`
}

// ReportAIResponse is the structured response from the AI report generation call.
type ReportAIResponse struct {
	ContentEn               string   `json:"content_en"`
	ContentEs               string   `json:"content_es"`
	AreasOfClarity          []string `json:"areas_of_clarity"`
	AreasOfClarityEs        []string `json:"areas_of_clarity_es"`
	AreasToDevelopFurther   []string `json:"areas_to_develop_further"`
	AreasToDevelopFurtherEs []string `json:"areas_to_develop_further_es"`
	Recommendation          string   `json:"recommendation"`
	RecommendationEs        string   `json:"recommendation_es"`
}
