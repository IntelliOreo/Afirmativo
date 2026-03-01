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

// CreateQuestionArea inserts a new in-progress area row.
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

// areaFromRow maps a sqlgen.QuestionArea to the domain QuestionArea type.
func areaFromRow(row sqlgen.QuestionArea) *QuestionArea {
	return &QuestionArea{
		ID:             uuidToString(row.ID),
		SessionCode:    row.SessionCode,
		Area:           row.Area,
		Status:         row.Status,
		QuestionsCount: int(row.QuestionsCount),
	}
}

// uuidToString formats a pgtype.UUID as a standard UUID string.
func uuidToString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	b := u.Bytes
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
