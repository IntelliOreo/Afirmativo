// PostgresStore implements Store backed by PostgreSQL via sqlgen.
package interview

import (
	"context"
	"errors"
	"fmt"

	"github.com/afirmativo/backend/internal/sqlgen"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore implements Store backed by PostgreSQL via sqlgen.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore creates a new PostgresStore.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

// CreateQuestionArea inserts a new area row.
// Returns nil, nil if the area already exists (ON CONFLICT DO NOTHING).
func (s *PostgresStore) CreateQuestionArea(ctx context.Context, sessionCode, area string) (*QuestionArea, error) {
	row, err := sqlgen.New(s.pool).CreateQuestionArea(ctx, sqlgen.CreateQuestionAreaParams{
		SessionCode: sessionCode,
		Area:        area,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("create question area: %w", err)
	}
	return areaFromRow(row), nil
}

// GetInProgressArea returns the current in-progress area, or nil if none.
func (s *PostgresStore) GetInProgressArea(ctx context.Context, sessionCode string) (*QuestionArea, error) {
	row, err := sqlgen.New(s.pool).GetInProgressArea(ctx, sessionCode)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get in-progress area: %w", err)
	}
	return areaFromRow(row), nil
}

// GetAreasBySession returns all question_area rows for a session.
func (s *PostgresStore) GetAreasBySession(ctx context.Context, sessionCode string) ([]QuestionArea, error) {
	rows, err := sqlgen.New(s.pool).GetAreasBySession(ctx, sessionCode)
	if err != nil {
		return nil, fmt.Errorf("get areas by session: %w", err)
	}
	areas := make([]QuestionArea, len(rows))
	for i, row := range rows {
		areas[i] = *areaFromRow(row)
	}
	return areas, nil
}

// SetAreaInProgress updates a pending or pre_addressed area to in_progress.
func (s *PostgresStore) SetAreaInProgress(ctx context.Context, sessionCode, area string) error {
	return sqlgen.New(s.pool).SetAreaInProgress(ctx, sqlgen.SetAreaInProgressParams{
		SessionCode: sessionCode,
		Area:        area,
	})
}

// IncrementAreaQuestions increments questions_count by 1.
func (s *PostgresStore) IncrementAreaQuestions(ctx context.Context, sessionCode, area string) error {
	return sqlgen.New(s.pool).IncrementAreaQuestions(ctx, sqlgen.IncrementAreaQuestionsParams{
		SessionCode: sessionCode,
		Area:        area,
	})
}

// CompleteArea marks an area as complete.
func (s *PostgresStore) CompleteArea(ctx context.Context, sessionCode, area string) error {
	return sqlgen.New(s.pool).CompleteArea(ctx, sqlgen.CompleteAreaParams{
		SessionCode: sessionCode,
		Area:        area,
	})
}

// MarkAreaInsufficient marks an area as insufficient.
func (s *PostgresStore) MarkAreaInsufficient(ctx context.Context, sessionCode, area string) error {
	return sqlgen.New(s.pool).MarkAreaInsufficient(ctx, sqlgen.MarkAreaInsufficientParams{
		SessionCode: sessionCode,
		Area:        area,
	})
}

// MarkAreaPreAddressed marks a pending area as pre_addressed with evidence.
func (s *PostgresStore) MarkAreaPreAddressed(ctx context.Context, sessionCode, area, evidence string) error {
	return sqlgen.New(s.pool).MarkAreaPreAddressed(ctx, sqlgen.MarkAreaPreAddressedParams{
		SessionCode:          sessionCode,
		Lower:                area, // sqlgen named this "Lower" due to LOWER($2) in query
		PreAddressedEvidence: pgtype.Text{String: evidence, Valid: true},
	})
}

// MarkAreaNotAssessed marks a pending/pre_addressed area as not_assessed.
func (s *PostgresStore) MarkAreaNotAssessed(ctx context.Context, sessionCode, area string) error {
	return sqlgen.New(s.pool).MarkAreaNotAssessed(ctx, sqlgen.MarkAreaNotAssessedParams{
		SessionCode: sessionCode,
		Area:        area,
	})
}

// SaveAnswer inserts a new answer row.
func (s *PostgresStore) SaveAnswer(ctx context.Context, params SaveAnswerParams) (*Answer, error) {
	row, err := sqlgen.New(s.pool).SaveAnswer(ctx, sqlgen.SaveAnswerParams{
		SessionCode:  params.SessionCode,
		Area:         params.Area,
		QuestionText: pgtype.Text{String: params.QuestionText, Valid: params.QuestionText != ""},
		TranscriptEs: pgtype.Text{String: params.TranscriptEs, Valid: params.TranscriptEs != ""},
		TranscriptEn: pgtype.Text{String: params.TranscriptEn, Valid: params.TranscriptEn != ""},
		AiEvaluation: params.AiEvaluation,
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

// ── Row mappers ─────────────────────────────────────────────────────

func areaFromRow(row sqlgen.QuestionArea) *QuestionArea {
	var evidence string
	if row.PreAddressedEvidence.Valid {
		evidence = row.PreAddressedEvidence.String
	}
	return &QuestionArea{
		ID:                   uuidToString(row.ID),
		SessionCode:          row.SessionCode,
		Area:                 row.Area,
		Status:               AreaStatus(row.Status),
		QuestionsCount:       int(row.QuestionsCount),
		PreAddressedEvidence: evidence,
	}
}

func answerFromRow(row sqlgen.Answer) *Answer {
	var questionText, transcriptEs, transcriptEn, sufficiency string
	if row.QuestionText.Valid {
		questionText = row.QuestionText.String
	}
	if row.TranscriptEs.Valid {
		transcriptEs = row.TranscriptEs.String
	}
	if row.TranscriptEn.Valid {
		transcriptEn = row.TranscriptEn.String
	}
	if row.Sufficiency.Valid {
		sufficiency = row.Sufficiency.String
	}
	var evalStr string
	if row.AiEvaluation != nil {
		evalStr = string(row.AiEvaluation)
	}
	return &Answer{
		ID:           uuidToString(row.ID),
		SessionCode:  row.SessionCode,
		Area:         row.Area,
		QuestionText: questionText,
		TranscriptEs: transcriptEs,
		TranscriptEn: transcriptEn,
		AiEvaluation: evalStr,
		Sufficiency:  sufficiency,
	}
}

func uuidToString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	b := u.Bytes
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
