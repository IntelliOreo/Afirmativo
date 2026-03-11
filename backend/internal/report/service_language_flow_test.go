package report

import (
	"context"
	"testing"

	"github.com/afirmativo/backend/internal/config"
)

type reportInterviewProviderStub struct {
	areas       []InterviewAreaSnapshot
	answers     []InterviewAnswerSnapshot
	answerCount int
}

func (s *reportInterviewProviderStub) GetAreasBySession(context.Context, string) ([]InterviewAreaSnapshot, error) {
	return s.areas, nil
}

func (s *reportInterviewProviderStub) GetAnswersBySession(context.Context, string) ([]InterviewAnswerSnapshot, error) {
	return s.answers, nil
}

func (s *reportInterviewProviderStub) GetAnswerCount(context.Context, string) (int, error) {
	return s.answerCount, nil
}

func TestProcessQueuedReport_LanguageTranscriptFlowAndBilingualReport(t *testing.T) {
	t.Parallel()

	const (
		sessionCode      = "AP-AAAA-BBBB"
		englishReport    = "English final analysis"
		spanishReport    = "Analisis final en espanol"
		recommendationEn = "Keep practicing with concise timelines."
		recommendationEs = "Siga practicando con lineas de tiempo concisas."
	)

	evaluation := &AnswerEvaluation{
		EvidenceSummary: "English evidence summary",
		Recommendation:  "follow_up",
	}

	tests := []struct {
		name              string
		preferredLanguage string
		answers           []InterviewAnswerSnapshot
		wantTranscript    string
	}{
		{
			name:              "english_user_uses_english_open_floor_transcript",
			preferredLanguage: "en",
			answers: []InterviewAnswerSnapshot{
				{Area: "protected_ground", QuestionText: "Main criterion question", TranscriptEn: "English answer body", TranscriptEs: "Respuesta en espanol", AIEvaluation: evaluation},
				{Area: "open_floor", QuestionText: "Open floor prompt #1", TranscriptEn: "EN open floor answer 1", TranscriptEs: "ES open floor answer 1"},
				{Area: "open_floor", QuestionText: "Open floor prompt #2", TranscriptEn: "EN open floor answer 2", TranscriptEs: "ES open floor answer 2"},
			},
			wantTranscript: "Q: Open floor prompt #1\nA: EN open floor answer 1\n\nQ: Open floor prompt #2\nA: EN open floor answer 2",
		},
		{
			name:              "spanish_user_uses_spanish_open_floor_transcript",
			preferredLanguage: "es",
			answers: []InterviewAnswerSnapshot{
				{Area: "protected_ground", QuestionText: "Pregunta principal", TranscriptEs: "Respuesta principal", AIEvaluation: evaluation},
				{Area: "open_floor", QuestionText: "Open floor prompt #1", TranscriptEn: "EN open floor answer 1", TranscriptEs: "ES open floor answer 1"},
				{Area: "open_floor", QuestionText: "Open floor prompt #2", TranscriptEn: "EN open floor answer 2", TranscriptEs: "ES open floor answer 2"},
			},
			wantTranscript: "Q: Open floor prompt #1\nA: ES open floor answer 1\n\nQ: Open floor prompt #2\nA: ES open floor answer 2",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var (
				readyReport        *Report
				capturedTranscript string
				capturedSummaries  []AreaSummary
			)

			store := &fakeReportStore{
				claimQueuedReportFn: func(context.Context, string) (*Report, error) {
					return &Report{SessionCode: sessionCode, Status: ReportStatusRunning, Attempts: 1}, nil
				},
				markReportReadyFn: func(_ context.Context, r *Report) error {
					copyReport := *r
					copyReport.AreasOfClarity = append([]string(nil), r.AreasOfClarity...)
					copyReport.AreasOfClarityEs = append([]string(nil), r.AreasOfClarityEs...)
					copyReport.AreasToDevelopFurther = append([]string(nil), r.AreasToDevelopFurther...)
					copyReport.AreasToDevelopFurtherEs = append([]string(nil), r.AreasToDevelopFurtherEs...)
					readyReport = &copyReport
					return nil
				},
			}

			interviews := &reportInterviewProviderStub{
				areas: []InterviewAreaSnapshot{
					{Area: "protected_ground", Status: "complete"},
					{Area: "open_floor", Status: "complete"},
				},
				answers:     tc.answers,
				answerCount: len(tc.answers),
			}

			sessions := &fakeReportSessionProvider{
				getSessionByCodeFn: func(context.Context, string) (*SessionInfo, error) {
					return &SessionInfo{
						SessionCode:        sessionCode,
						Status:             "completed",
						PreferredLanguage:  tc.preferredLanguage,
						InterviewStartedAt: 1_000,
						EndedAt:            1_600,
					}, nil
				},
			}

			ai := &fakeReportAIClient{
				generateReportFn: func(_ context.Context, summaries []AreaSummary, transcript string) (*ReportAIResponse, error) {
					capturedSummaries = append([]AreaSummary(nil), summaries...)
					capturedTranscript = transcript
					return &ReportAIResponse{
						ContentEn:               englishReport,
						ContentEs:               spanishReport,
						AreasOfClarity:          []string{"Clear chronology in key events"},
						AreasOfClarityEs:        []string{"Cronologia clara en eventos clave"},
						AreasToDevelopFurther:   []string{"Clarify exact dates for each event"},
						AreasToDevelopFurtherEs: []string{"Aclare las fechas exactas de cada evento"},
						Recommendation:          recommendationEn,
						RecommendationEs:        recommendationEs,
					}, nil
				},
			}

			areaConfigs := []config.AreaConfig{
				{Slug: "protected_ground", Label: "Protected ground"},
				{Slug: "open_floor", Label: "Open floor"},
			}

			svc := NewService(store, interviews, sessions, ai, areaConfigs)
			svc.processQueuedReport(context.Background(), sessionCode)

			if capturedTranscript != tc.wantTranscript {
				t.Fatalf("AI transcript = %q, want %q", capturedTranscript, tc.wantTranscript)
			}
			if readyReport == nil {
				t.Fatalf("expected ready report to be persisted")
			}
			if readyReport.Status != ReportStatusReady {
				t.Fatalf("status = %q, want ready", readyReport.Status)
			}
			if readyReport.ContentEn != englishReport || readyReport.ContentEs != spanishReport {
				t.Fatalf("content mismatch: %#v", readyReport)
			}
			if readyReport.Recommendation != recommendationEn || readyReport.RecommendationEs != recommendationEs {
				t.Fatalf("recommendation mismatch: %#v", readyReport)
			}
			if len(capturedSummaries) != 2 {
				t.Fatalf("area summaries count = %d, want 2", len(capturedSummaries))
			}
		})
	}
}
