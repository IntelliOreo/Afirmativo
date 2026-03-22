package interview

import (
	"context"
	"fmt"

	"github.com/afirmativo/backend/internal/sqlgen"
	"github.com/jackc/pgx/v5/pgtype"
)

// SaveAnswer inserts a new answer row.
func (s *PostgresStore) SaveAnswer(ctx context.Context, params SaveAnswerParams) (*Answer, error) {
	row, err := sqlgen.New(s.pool).SaveAnswer(ctx, sqlgen.SaveAnswerParams{
		SessionCode:  params.SessionCode,
		Area:         params.Area,
		QuestionText: pgtype.Text{String: params.QuestionText, Valid: params.QuestionText != ""},
		TranscriptEs: pgtype.Text{String: params.TranscriptEs, Valid: params.TranscriptEs != ""},
		TranscriptEn: pgtype.Text{String: params.TranscriptEn, Valid: params.TranscriptEn != ""},
		AiEvaluation: params.AIEvaluationJSON,
		Sufficiency:  pgtype.Text{String: params.Sufficiency, Valid: params.Sufficiency != ""},
	})
	if err != nil {
		return nil, fmt.Errorf("save answer: %w", err)
	}
	return answerFromRow(row), nil
}

// GetAnswersBySession returns all answers for a session ordered by created_at.
func (s *PostgresStore) GetAnswersBySession(ctx context.Context, sessionCode string) ([]Answer, error) {
	rows, err := sqlgen.New(s.pool).GetAnswersBySession(ctx, sessionCode)
	if err != nil {
		return nil, fmt.Errorf("get answers by session: %w", err)
	}
	answers := make([]Answer, len(rows))
	for i, row := range rows {
		answers[i] = *answerFromRow(row)
	}
	return answers, nil
}

// GetAnswerCount returns the number of answers for a session.
func (s *PostgresStore) GetAnswerCount(ctx context.Context, sessionCode string) (int, error) {
	count, err := sqlgen.New(s.pool).GetAnswerCount(ctx, sessionCode)
	if err != nil {
		return 0, fmt.Errorf("get answer count: %w", err)
	}
	return int(count), nil
}
