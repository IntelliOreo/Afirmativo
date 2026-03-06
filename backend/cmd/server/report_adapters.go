package main

import (
	"context"

	"github.com/afirmativo/backend/internal/interview"
	"github.com/afirmativo/backend/internal/report"
	"github.com/afirmativo/backend/internal/session"
)

// interviewDataAdapter adapts interview.PostgresStore to report.InterviewDataProvider.
type interviewDataAdapter struct {
	store *interview.PostgresStore
}

func (a *interviewDataAdapter) GetAreasBySession(ctx context.Context, sessionCode string) ([]report.QuestionAreaRow, error) {
	areas, err := a.store.GetAreasBySession(ctx, sessionCode)
	if err != nil {
		return nil, err
	}
	result := make([]report.QuestionAreaRow, len(areas))
	for i, area := range areas {
		result[i] = report.QuestionAreaRow{
			Area:                 area.Area,
			Status:               string(area.Status),
			PreAddressedEvidence: area.PreAddressedEvidence,
		}
	}
	return result, nil
}

func (a *interviewDataAdapter) GetAnswersBySession(ctx context.Context, sessionCode string) ([]report.AnswerRow, error) {
	answers, err := a.store.GetAnswersBySession(ctx, sessionCode)
	if err != nil {
		return nil, err
	}
	result := make([]report.AnswerRow, len(answers))
	for i, ans := range answers {
		result[i] = report.AnswerRow{
			Area:         ans.Area,
			QuestionText: ans.QuestionText,
			TranscriptEs: ans.TranscriptEs,
			TranscriptEn: ans.TranscriptEn,
			AiEvaluation: ans.AiEvaluation,
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
	store *session.PostgresStore
}

func (a *sessionDataAdapter) GetSessionByCode(ctx context.Context, sessionCode string) (*report.SessionInfo, error) {
	sess, err := a.store.GetSessionByCode(ctx, sessionCode)
	if err != nil {
		return nil, err
	}
	info := &report.SessionInfo{
		SessionCode:       sess.SessionCode,
		Status:            sess.Status,
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
