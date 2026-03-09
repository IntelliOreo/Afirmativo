package main

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/afirmativo/backend/internal/interview"
	"github.com/afirmativo/backend/internal/report"
	"github.com/afirmativo/backend/internal/session"
)

type interviewDataSource interface {
	GetAreasBySession(ctx context.Context, sessionCode string) ([]interview.QuestionArea, error)
	GetAnswersBySession(ctx context.Context, sessionCode string) ([]interview.Answer, error)
	GetAnswerCount(ctx context.Context, sessionCode string) (int, error)
}

type sessionSource interface {
	GetSessionByCode(ctx context.Context, sessionCode string) (*session.Session, error)
}

// interviewDataAdapter adapts interview data sources to report.InterviewDataProvider.
type interviewDataAdapter struct {
	store interviewDataSource
}

func (a *interviewDataAdapter) GetAreasBySession(ctx context.Context, sessionCode string) ([]report.InterviewAreaSnapshot, error) {
	areas, err := a.store.GetAreasBySession(ctx, sessionCode)
	if err != nil {
		return nil, err
	}
	result := make([]report.InterviewAreaSnapshot, len(areas))
	for i, area := range areas {
		result[i] = report.InterviewAreaSnapshot{
			Area:                 area.Area,
			Status:               string(area.Status),
			PreAddressedEvidence: area.PreAddressedEvidence,
		}
	}
	return result, nil
}

func (a *interviewDataAdapter) GetAnswersBySession(ctx context.Context, sessionCode string) ([]report.InterviewAnswerSnapshot, error) {
	answers, err := a.store.GetAnswersBySession(ctx, sessionCode)
	if err != nil {
		return nil, err
	}
	result := make([]report.InterviewAnswerSnapshot, len(answers))
	for i, ans := range answers {
		var eval *report.AnswerEvaluation
		if ans.AIEvaluationJSON != "" {
			var parsed interview.Evaluation
			if err := json.Unmarshal([]byte(ans.AIEvaluationJSON), &parsed); err != nil {
				slog.Warn("failed to parse evaluation for report adapter", "area", ans.Area, "error", err)
			} else {
				eval = &report.AnswerEvaluation{
					EvidenceSummary: parsed.CurrentCriterion.EvidenceSummary,
					Recommendation:  string(parsed.CurrentCriterion.Recommendation),
				}
			}
		}
		result[i] = report.InterviewAnswerSnapshot{
			Area:         ans.Area,
			QuestionText: ans.QuestionText,
			TranscriptEs: ans.TranscriptEs,
			TranscriptEn: ans.TranscriptEn,
			AIEvaluation: eval,
			Sufficiency:  ans.Sufficiency,
		}
	}
	return result, nil
}

func (a *interviewDataAdapter) GetAnswerCount(ctx context.Context, sessionCode string) (int, error) {
	return a.store.GetAnswerCount(ctx, sessionCode)
}

// sessionDataAdapter adapts session.PostgresStore to report.SessionProvider.
type sessionDataAdapter struct {
	store sessionSource
}

func (a *sessionDataAdapter) GetSessionByCode(ctx context.Context, sessionCode string) (*report.SessionInfo, error) {
	sess, err := a.store.GetSessionByCode(ctx, sessionCode)
	if err != nil {
		return nil, err
	}
	info := &report.SessionInfo{
		SessionCode:       sess.SessionCode,
		Status:            string(sess.Status),
		PreferredLanguage: sess.PreferredLanguage,
	}
	if sess.InterviewStartedAt != nil {
		info.InterviewStartedAt = sess.InterviewStartedAt.Unix()
	}
	if sess.EndedAt != nil {
		info.EndedAt = sess.EndedAt.Unix()
	}
	return info, nil
}
