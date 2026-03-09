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

func TestServiceGetOrGenerateReport_LanguageTranscriptFlowAndBilingualReport(t *testing.T) {
	t.Parallel()

	const (
		sessionCode      = "AP-AAAA-BBBB"
		englishReport    = "English final analysis"
		spanishReport    = "Analisis final en espanol"
		recommendationEn = "Keep practicing with concise timelines."
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
				{
					Area:         "protected_ground",
					QuestionText: "Main criterion question",
					TranscriptEn: "English answer body",
					TranscriptEs: "Respuesta en espanol",
					AIEvaluation: evaluation,
				},
				{
					Area:         "open_floor",
					QuestionText: "Open floor prompt #1",
					TranscriptEn: "EN open floor answer 1",
					TranscriptEs: "ES open floor answer 1",
				},
				{
					Area:         "open_floor",
					QuestionText: "Open floor prompt #2",
					TranscriptEn: "EN open floor answer 2",
					TranscriptEs: "ES open floor answer 2",
				},
			},
			wantTranscript: "Q: Open floor prompt #1\nA: EN open floor answer 1\n\nQ: Open floor prompt #2\nA: EN open floor answer 2",
		},
		{
			name:              "spanish_user_uses_spanish_open_floor_transcript_but_keeps_english_analysis",
			preferredLanguage: "es",
			answers: []InterviewAnswerSnapshot{
				{
					Area:         "protected_ground",
					QuestionText: "Pregunta principal",
					TranscriptEs: "Respuesta principal",
					AIEvaluation: evaluation,
				},
				{
					Area:         "open_floor",
					QuestionText: "Open floor prompt #1",
					TranscriptEn: "EN open floor answer 1",
					TranscriptEs: "ES open floor answer 1",
				},
				{
					Area:         "open_floor",
					QuestionText: "Open floor prompt #2",
					TranscriptEn: "EN open floor answer 2",
					TranscriptEs: "ES open floor answer 2",
				},
			},
			wantTranscript: "Q: Open floor prompt #1\nA: ES open floor answer 1\n\nQ: Open floor prompt #2\nA: ES open floor answer 2",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var (
				createdReport      *Report
				updatedReport      *Report
				capturedTranscript string
				capturedSummaries  []AreaSummary
			)

			store := &fakeReportStore{
				getReportBySessionFn: func(context.Context, string) (*Report, error) {
					return nil, nil
				},
				createReportFn: func(_ context.Context, r *Report) error {
					reportCopy := *r
					createdReport = &reportCopy
					return nil
				},
				updateReportFn: func(_ context.Context, r *Report) error {
					reportCopy := *r
					reportCopy.AreasOfClarity = append([]string(nil), r.AreasOfClarity...)
					reportCopy.AreasToDevelopFurther = append([]string(nil), r.AreasToDevelopFurther...)
					updatedReport = &reportCopy
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
						ContentEn:             englishReport,
						ContentEs:             spanishReport,
						AreasOfClarity:        []string{"Clear chronology in key events"},
						AreasToDevelopFurther: []string{"Clarify exact dates for each event"},
						Recommendation:        recommendationEn,
					}, nil
				},
			}

			areaConfigs := []config.AreaConfig{
				{Slug: "protected_ground", Label: "Protected ground"},
				{Slug: "open_floor", Label: "Open floor"},
			}

			svc := NewService(store, interviews, sessions, ai, areaConfigs)

			got, err := svc.GetOrGenerateReport(context.Background(), sessionCode)
			if err != nil {
				t.Fatalf("GetOrGenerateReport() error = %v", err)
			}
			if got == nil {
				t.Fatalf("GetOrGenerateReport() = nil, want non-nil report")
			}
			if got.Status != "ready" {
				t.Fatalf("report status = %q, want ready", got.Status)
			}
			if capturedTranscript != tc.wantTranscript {
				t.Fatalf("AI transcript = %q, want %q", capturedTranscript, tc.wantTranscript)
			}
			if createdReport == nil || createdReport.Status != "generating" {
				t.Fatalf("created report = %#v, want status generating placeholder", createdReport)
			}
			if updatedReport == nil {
				t.Fatalf("expected report to be persisted via UpdateReport")
			}
			if updatedReport.ContentEn != englishReport {
				t.Fatalf("content_en = %q, want %q", updatedReport.ContentEn, englishReport)
			}
			if updatedReport.ContentEs != spanishReport {
				t.Fatalf("content_es = %q, want %q", updatedReport.ContentEs, spanishReport)
			}
			if updatedReport.Recommendation != recommendationEn {
				t.Fatalf("recommendation = %q, want %q", updatedReport.Recommendation, recommendationEn)
			}
			if len(updatedReport.AreasOfClarity) == 0 || len(updatedReport.AreasToDevelopFurther) == 0 {
				t.Fatalf("english analysis bullets should be preserved: clarity=%v develop=%v", updatedReport.AreasOfClarity, updatedReport.AreasToDevelopFurther)
			}
			if len(capturedSummaries) != 2 {
				t.Fatalf("area summaries count = %d, want 2", len(capturedSummaries))
			}
			if capturedSummaries[0].EvidenceSummary != "English evidence summary" {
				t.Fatalf("summary evidence = %q, want English evidence summary", capturedSummaries[0].EvidenceSummary)
			}
		})
	}
}
