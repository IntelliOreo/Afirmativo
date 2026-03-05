// Package report handles report retrieval and generation.
// This file defines the Report domain type — no infrastructure imports.
package report

import "errors"

var (
	ErrSessionNotFound     = errors.New("report session not found")
	ErrSessionNotCompleted = errors.New("report session not completed")
)

// Report represents a completed assessment report.
type Report struct {
	SessionCode     string
	Status          string // "generating", "ready", "failed"
	ContentEn       string
	ContentEs       string
	Strengths       []string
	Weaknesses      []string
	Recommendation  string
	QuestionCount   int
	DurationMinutes int
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
	ContentEn      string   `json:"content_en"`
	ContentEs      string   `json:"content_es"`
	Strengths      []string `json:"strengths"`
	Weaknesses     []string `json:"weaknesses"`
	Recommendation string   `json:"recommendation"`
}
