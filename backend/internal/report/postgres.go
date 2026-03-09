// PostgresStore implements Store backed by PostgreSQL via sqlgen.
package report

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/afirmativo/backend/internal/sqlgen"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore implements Store for the reports table.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore creates a new PostgresStore.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

// GetReportBySession returns the report for a session, or nil if not found.
func (s *PostgresStore) GetReportBySession(ctx context.Context, sessionCode string) (*Report, error) {
	row, err := sqlgen.New(s.pool).GetReportBySession(ctx, sessionCode)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get report: %w", err)
	}
	return reportFromRow(row), nil
}

// CreateReport inserts a new report row.
func (s *PostgresStore) CreateReport(ctx context.Context, r *Report) error {
	strengthsJSON, _ := json.Marshal(r.AreasOfClarity)
	strengthsEsJSON, _ := json.Marshal(r.AreasOfClarityEs)
	weaknessesJSON, _ := json.Marshal(r.AreasToDevelopFurther)
	weaknessesEsJSON, _ := json.Marshal(r.AreasToDevelopFurtherEs)

	_, err := sqlgen.New(s.pool).CreateReport(ctx, sqlgen.CreateReportParams{
		SessionCode:      r.SessionCode,
		Status:           string(r.Status),
		ContentEn:        pgtype.Text{String: r.ContentEn, Valid: r.ContentEn != ""},
		ContentEs:        pgtype.Text{String: r.ContentEs, Valid: r.ContentEs != ""},
		Strengths:        strengthsJSON,
		StrengthsEs:      strengthsEsJSON,
		Weaknesses:       weaknessesJSON,
		WeaknessesEs:     weaknessesEsJSON,
		Recommendation:   pgtype.Text{String: r.Recommendation, Valid: r.Recommendation != ""},
		RecommendationEs: pgtype.Text{String: r.RecommendationEs, Valid: r.RecommendationEs != ""},
		QuestionCount:    int32(r.QuestionCount),
		DurationMinutes:  int32(r.DurationMinutes),
	})
	if err != nil {
		return fmt.Errorf("create report: %w", err)
	}
	return nil
}

// UpdateReport updates a report with the generated content.
func (s *PostgresStore) UpdateReport(ctx context.Context, r *Report) error {
	strengthsJSON, _ := json.Marshal(r.AreasOfClarity)
	strengthsEsJSON, _ := json.Marshal(r.AreasOfClarityEs)
	weaknessesJSON, _ := json.Marshal(r.AreasToDevelopFurther)
	weaknessesEsJSON, _ := json.Marshal(r.AreasToDevelopFurtherEs)

	err := sqlgen.New(s.pool).UpdateReport(ctx, sqlgen.UpdateReportParams{
		SessionCode:      r.SessionCode,
		Status:           string(r.Status),
		ContentEn:        pgtype.Text{String: r.ContentEn, Valid: r.ContentEn != ""},
		ContentEs:        pgtype.Text{String: r.ContentEs, Valid: r.ContentEs != ""},
		Strengths:        strengthsJSON,
		StrengthsEs:      strengthsEsJSON,
		Weaknesses:       weaknessesJSON,
		WeaknessesEs:     weaknessesEsJSON,
		Recommendation:   pgtype.Text{String: r.Recommendation, Valid: r.Recommendation != ""},
		RecommendationEs: pgtype.Text{String: r.RecommendationEs, Valid: r.RecommendationEs != ""},
		QuestionCount:    int32(r.QuestionCount),
		DurationMinutes:  int32(r.DurationMinutes),
	})
	if err != nil {
		return fmt.Errorf("update report: %w", err)
	}
	return nil
}

// ── Row mapper ──────────────────────────────────────────────────────

func reportFromRow(row sqlgen.Report) *Report {
	r := &Report{
		SessionCode:     row.SessionCode,
		Status:          ReportStatus(row.Status),
		QuestionCount:   int(row.QuestionCount),
		DurationMinutes: int(row.DurationMinutes),
	}
	if row.ContentEn.Valid {
		r.ContentEn = row.ContentEn.String
	}
	if row.ContentEs.Valid {
		r.ContentEs = row.ContentEs.String
	}
	if row.Recommendation.Valid {
		r.Recommendation = row.Recommendation.String
	}
	if row.RecommendationEs.Valid {
		r.RecommendationEs = row.RecommendationEs.String
	}
	if row.Strengths != nil {
		_ = json.Unmarshal(row.Strengths, &r.AreasOfClarity)
	}
	if r.AreasOfClarity == nil {
		r.AreasOfClarity = []string{}
	}
	if row.StrengthsEs != nil {
		_ = json.Unmarshal(row.StrengthsEs, &r.AreasOfClarityEs)
	}
	if r.AreasOfClarityEs == nil {
		r.AreasOfClarityEs = []string{}
	}
	if row.Weaknesses != nil {
		_ = json.Unmarshal(row.Weaknesses, &r.AreasToDevelopFurther)
	}
	if r.AreasToDevelopFurther == nil {
		r.AreasToDevelopFurther = []string{}
	}
	if row.WeaknessesEs != nil {
		_ = json.Unmarshal(row.WeaknessesEs, &r.AreasToDevelopFurtherEs)
	}
	if r.AreasToDevelopFurtherEs == nil {
		r.AreasToDevelopFurtherEs = []string{}
	}
	return r
}
