package report

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

func (s *PostgresStore) GetReportBySession(ctx context.Context, sessionCode string) (*Report, error) {
	row := s.pool.QueryRow(ctx, `
SELECT session_code, status, content_en, content_es, strengths, strengths_es, weaknesses, weaknesses_es,
       recommendation, recommendation_es, question_count, duration_minutes, error_code, error_message,
       attempts, started_at, completed_at
  FROM reports
 WHERE session_code = $1`, sessionCode)

	report, err := scanReport(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get report: %w", err)
	}
	return report, nil
}

func (s *PostgresStore) CreateReport(ctx context.Context, r *Report) error {
	strengthsJSON, _ := json.Marshal(r.AreasOfClarity)
	strengthsEsJSON, _ := json.Marshal(r.AreasOfClarityEs)
	weaknessesJSON, _ := json.Marshal(r.AreasToDevelopFurther)
	weaknessesEsJSON, _ := json.Marshal(r.AreasToDevelopFurtherEs)

	_, err := s.pool.Exec(ctx, `
INSERT INTO reports (
    session_code, status, content_en, content_es, strengths, strengths_es, weaknesses, weaknesses_es,
    recommendation, recommendation_es, question_count, duration_minutes, error_code, error_message,
    attempts, started_at, completed_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)`,
		r.SessionCode,
		string(r.Status),
		nullText(r.ContentEn),
		nullText(r.ContentEs),
		strengthsJSON,
		strengthsEsJSON,
		weaknessesJSON,
		weaknessesEsJSON,
		nullText(r.Recommendation),
		nullText(r.RecommendationEs),
		r.QuestionCount,
		r.DurationMinutes,
		nullText(r.ErrorCode),
		nullText(r.ErrorMessage),
		r.Attempts,
		nullTime(r.StartedAt),
		nullTime(r.CompletedAt),
	)
	if err != nil {
		return fmt.Errorf("create report: %w", err)
	}
	return nil
}

func (s *PostgresStore) SetReportQueued(ctx context.Context, sessionCode string, resetAttempts bool) error {
	attemptsExpr := "attempts"
	if resetAttempts {
		attemptsExpr = "0"
	}

	tag, err := s.pool.Exec(ctx, fmt.Sprintf(`
UPDATE reports
   SET status = 'queued',
       error_code = NULL,
       error_message = NULL,
       started_at = NULL,
       completed_at = NULL,
       attempts = %s,
       updated_at = now()
 WHERE session_code = $1`, attemptsExpr), sessionCode)
	if err != nil {
		return fmt.Errorf("set report queued: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrReportNotStarted
	}
	return nil
}

func (s *PostgresStore) ClaimQueuedReport(ctx context.Context, sessionCode string) (*Report, error) {
	row := s.pool.QueryRow(ctx, `
UPDATE reports
   SET status = 'running',
       attempts = attempts + 1,
       started_at = now(),
       completed_at = NULL,
       updated_at = now()
 WHERE session_code = $1
   AND status = 'queued'
 RETURNING session_code, status, content_en, content_es, strengths, strengths_es, weaknesses, weaknesses_es,
           recommendation, recommendation_es, question_count, duration_minutes, error_code, error_message,
           attempts, started_at, completed_at`, sessionCode)

	report, err := scanReport(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("claim queued report: %w", err)
	}
	return report, nil
}

func (s *PostgresStore) ListQueuedReportSessionCodes(ctx context.Context, limit int) ([]string, error) {
	if limit <= 0 {
		return []string{}, nil
	}

	rows, err := s.pool.Query(ctx, `
SELECT session_code
  FROM reports
 WHERE status = 'queued'
 ORDER BY updated_at ASC
 LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list queued reports: %w", err)
	}
	defer rows.Close()

	sessionCodes := make([]string, 0, limit)
	for rows.Next() {
		var sessionCode string
		if err := rows.Scan(&sessionCode); err != nil {
			return nil, fmt.Errorf("scan queued report session code: %w", err)
		}
		sessionCodes = append(sessionCodes, sessionCode)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate queued reports: %w", err)
	}
	return sessionCodes, nil
}

func (s *PostgresStore) RequeueStaleRunningReports(ctx context.Context, staleBefore time.Time) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
UPDATE reports
   SET status = 'queued',
       started_at = NULL,
       completed_at = NULL,
       updated_at = now()
 WHERE status = 'running'
   AND started_at IS NOT NULL
   AND started_at < $1`, staleBefore.UTC())
	if err != nil {
		return 0, fmt.Errorf("requeue stale running reports: %w", err)
	}
	return tag.RowsAffected(), nil
}

func (s *PostgresStore) MarkReportReady(ctx context.Context, r *Report) error {
	strengthsJSON, _ := json.Marshal(r.AreasOfClarity)
	strengthsEsJSON, _ := json.Marshal(r.AreasOfClarityEs)
	weaknessesJSON, _ := json.Marshal(r.AreasToDevelopFurther)
	weaknessesEsJSON, _ := json.Marshal(r.AreasToDevelopFurtherEs)

	tag, err := s.pool.Exec(ctx, `
UPDATE reports
   SET status = 'ready',
       content_en = $2,
       content_es = $3,
       strengths = $4,
       strengths_es = $5,
       weaknesses = $6,
       weaknesses_es = $7,
       recommendation = $8,
       recommendation_es = $9,
       question_count = $10,
       duration_minutes = $11,
       error_code = NULL,
       error_message = NULL,
       completed_at = now(),
       updated_at = now()
 WHERE session_code = $1`,
		r.SessionCode,
		nullText(r.ContentEn),
		nullText(r.ContentEs),
		strengthsJSON,
		strengthsEsJSON,
		weaknessesJSON,
		weaknessesEsJSON,
		nullText(r.Recommendation),
		nullText(r.RecommendationEs),
		r.QuestionCount,
		r.DurationMinutes,
	)
	if err != nil {
		return fmt.Errorf("mark report ready: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrReportNotStarted
	}
	return nil
}

func (s *PostgresStore) MarkReportFailed(ctx context.Context, sessionCode, errorCode, errorMessage string) error {
	tag, err := s.pool.Exec(ctx, `
UPDATE reports
   SET status = 'failed',
       error_code = $2,
       error_message = $3,
       completed_at = now(),
       updated_at = now()
 WHERE session_code = $1`,
		sessionCode,
		nullText(errorCode),
		nullText(errorMessage),
	)
	if err != nil {
		return fmt.Errorf("mark report failed: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrReportNotStarted
	}
	return nil
}

func scanReport(row interface{ Scan(...any) error }) (*Report, error) {
	var (
		sessionCode      string
		status           string
		contentEn        pgtype.Text
		contentEs        pgtype.Text
		strengths        []byte
		strengthsEs      []byte
		weaknesses       []byte
		weaknessesEs     []byte
		recommendation   pgtype.Text
		recommendationEs pgtype.Text
		questionCount    int32
		durationMinutes  int32
		errorCode        pgtype.Text
		errorMessage     pgtype.Text
		attempts         int32
		startedAt        pgtype.Timestamptz
		completedAt      pgtype.Timestamptz
	)

	if err := row.Scan(
		&sessionCode,
		&status,
		&contentEn,
		&contentEs,
		&strengths,
		&strengthsEs,
		&weaknesses,
		&weaknessesEs,
		&recommendation,
		&recommendationEs,
		&questionCount,
		&durationMinutes,
		&errorCode,
		&errorMessage,
		&attempts,
		&startedAt,
		&completedAt,
	); err != nil {
		return nil, err
	}

	report := &Report{
		SessionCode:     sessionCode,
		Status:          ReportStatus(status),
		QuestionCount:   int(questionCount),
		DurationMinutes: int(durationMinutes),
		Attempts:        int(attempts),
	}
	if contentEn.Valid {
		report.ContentEn = contentEn.String
	}
	if contentEs.Valid {
		report.ContentEs = contentEs.String
	}
	if recommendation.Valid {
		report.Recommendation = recommendation.String
	}
	if recommendationEs.Valid {
		report.RecommendationEs = recommendationEs.String
	}
	if errorCode.Valid {
		report.ErrorCode = errorCode.String
	}
	if errorMessage.Valid {
		report.ErrorMessage = errorMessage.String
	}
	if startedAt.Valid {
		value := startedAt.Time
		report.StartedAt = &value
	}
	if completedAt.Valid {
		value := completedAt.Time
		report.CompletedAt = &value
	}
	if strengths != nil {
		_ = json.Unmarshal(strengths, &report.AreasOfClarity)
	}
	if report.AreasOfClarity == nil {
		report.AreasOfClarity = []string{}
	}
	if strengthsEs != nil {
		_ = json.Unmarshal(strengthsEs, &report.AreasOfClarityEs)
	}
	if report.AreasOfClarityEs == nil {
		report.AreasOfClarityEs = []string{}
	}
	if weaknesses != nil {
		_ = json.Unmarshal(weaknesses, &report.AreasToDevelopFurther)
	}
	if report.AreasToDevelopFurther == nil {
		report.AreasToDevelopFurther = []string{}
	}
	if weaknessesEs != nil {
		_ = json.Unmarshal(weaknessesEs, &report.AreasToDevelopFurtherEs)
	}
	if report.AreasToDevelopFurtherEs == nil {
		report.AreasToDevelopFurtherEs = []string{}
	}
	return report, nil
}

func nullText(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: strings.TrimSpace(value) != ""}
}

func nullTime(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}
