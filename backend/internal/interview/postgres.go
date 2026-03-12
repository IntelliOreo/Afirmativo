// PostgresStore implements Store backed by PostgreSQL via sqlgen.
package interview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

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
		ID:               uuidToString(row.ID),
		SessionCode:      row.SessionCode,
		Area:             row.Area,
		QuestionText:     questionText,
		TranscriptEs:     transcriptEs,
		TranscriptEn:     transcriptEn,
		AIEvaluationJSON: evalStr,
		Sufficiency:      sufficiency,
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

func issuedQuestionToDBFields(issuedQuestion *IssuedQuestion) (pgtype.Text, pgtype.Text, pgtype.Text, pgtype.Text, pgtype.Timestamptz, pgtype.Timestamptz) {
	if issuedQuestion == nil {
		return pgtype.Text{}, pgtype.Text{}, pgtype.Text{}, pgtype.Text{}, pgtype.Timestamptz{}, pgtype.Timestamptz{}
	}
	return pgtype.Text{String: issuedQuestion.Question.TextEs, Valid: issuedQuestion.Question.TextEs != ""},
		pgtype.Text{String: issuedQuestion.Question.TextEn, Valid: issuedQuestion.Question.TextEn != ""},
		pgtype.Text{String: issuedQuestion.Question.Area, Valid: issuedQuestion.Question.Area != ""},
		pgtype.Text{String: string(issuedQuestion.Question.Kind), Valid: issuedQuestion.Question.Kind != ""},
		pgtype.Timestamptz{Time: issuedQuestion.IssuedAt.UTC(), Valid: true},
		pgtype.Timestamptz{Time: issuedQuestion.AnswerDeadlineAt.UTC(), Valid: true}
}

func issuedQuestionFromDB(
	expectedTurnID string,
	questionNumber int,
	textEs pgtype.Text,
	textEn pgtype.Text,
	area pgtype.Text,
	kind pgtype.Text,
	issuedAt pgtype.Timestamptz,
	answerDeadlineAt pgtype.Timestamptz,
) *IssuedQuestion {
	if strings.TrimSpace(expectedTurnID) == "" || !kind.Valid || !issuedAt.Valid || !answerDeadlineAt.Valid {
		return nil
	}
	return &IssuedQuestion{
		Question: Question{
			TextEs:         textEs.String,
			TextEn:         textEn.String,
			Area:           area.String,
			Kind:           QuestionKind(kind.String),
			TurnID:         expectedTurnID,
			QuestionNumber: questionNumber,
			TotalQuestions: EstimatedTotalQuestions,
		},
		IssuedAt:         issuedAt.Time.UTC(),
		AnswerDeadlineAt: answerDeadlineAt.Time.UTC(),
	}
}

// GetFlowState returns the persisted interview flow pointer for a session.
func (s *PostgresStore) GetFlowState(ctx context.Context, sessionCode string) (*FlowState, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT flow_step,
		        COALESCE(expected_turn_id, ''),
		        display_question_number,
		        active_question_text_es,
		        active_question_text_en,
		        active_question_area,
		        active_question_kind,
		        active_question_issued_at,
		        active_answer_deadline_at
		 FROM sessions
		 WHERE session_code = $1`,
		sessionCode,
	)

	var step string
	var turnID string
	var questionNumber int
	var textEs pgtype.Text
	var textEn pgtype.Text
	var area pgtype.Text
	var kind pgtype.Text
	var issuedAt pgtype.Timestamptz
	var answerDeadlineAt pgtype.Timestamptz
	if err := row.Scan(
		&step,
		&turnID,
		&questionNumber,
		&textEs,
		&textEn,
		&area,
		&kind,
		&issuedAt,
		&answerDeadlineAt,
	); err != nil {
		return nil, fmt.Errorf("get flow state: %w", err)
	}

	return &FlowState{
		Step:           FlowStep(step),
		ExpectedTurnID: turnID,
		QuestionNumber: questionNumber,
		ActiveQuestion: issuedQuestionFromDB(
			turnID,
			questionNumber,
			textEs,
			textEn,
			area,
			kind,
			issuedAt,
			answerDeadlineAt,
		),
	}, nil
}

// PrepareDisclaimerStep forces the flow pointer to disclaimer and sets turn id.
func (s *PostgresStore) PrepareDisclaimerStep(ctx context.Context, sessionCode string, issuedQuestion *IssuedQuestion) (*FlowState, error) {
	textEs, textEn, area, kind, issuedAt, answerDeadlineAt := issuedQuestionToDBFields(issuedQuestion)
	row := s.pool.QueryRow(ctx,
		`UPDATE sessions
		 SET flow_step = 'disclaimer',
		     expected_turn_id = $2,
		     active_question_text_es = $3,
		     active_question_text_en = $4,
		     active_question_area = $5,
		     active_question_kind = $6,
		     active_question_issued_at = $7,
		     active_answer_deadline_at = $8
		 WHERE session_code = $1
		 RETURNING flow_step,
		           COALESCE(expected_turn_id, ''),
		           display_question_number,
		           active_question_text_es,
		           active_question_text_en,
		           active_question_area,
		           active_question_kind,
		           active_question_issued_at,
		           active_answer_deadline_at`,
		sessionCode,
		issuedQuestion.Question.TurnID,
		textEs,
		textEn,
		area,
		kind,
		issuedAt,
		answerDeadlineAt,
	)

	var step string
	var expectedTurnID string
	var questionNumber int
	var persistedTextEs pgtype.Text
	var persistedTextEn pgtype.Text
	var persistedArea pgtype.Text
	var persistedKind pgtype.Text
	var persistedIssuedAt pgtype.Timestamptz
	var persistedAnswerDeadlineAt pgtype.Timestamptz
	if err := row.Scan(
		&step,
		&expectedTurnID,
		&questionNumber,
		&persistedTextEs,
		&persistedTextEn,
		&persistedArea,
		&persistedKind,
		&persistedIssuedAt,
		&persistedAnswerDeadlineAt,
	); err != nil {
		return nil, fmt.Errorf("prepare disclaimer step: %w", err)
	}

	return &FlowState{
		Step:           FlowStep(step),
		ExpectedTurnID: expectedTurnID,
		QuestionNumber: questionNumber,
		ActiveQuestion: issuedQuestionFromDB(
			expectedTurnID,
			questionNumber,
			persistedTextEs,
			persistedTextEn,
			persistedArea,
			persistedKind,
			persistedIssuedAt,
			persistedAnswerDeadlineAt,
		),
	}, nil
}

// PrepareReadinessStep forces the flow pointer to readiness and sets turn id.
func (s *PostgresStore) PrepareReadinessStep(ctx context.Context, sessionCode string, issuedQuestion *IssuedQuestion) (*FlowState, error) {
	textEs, textEn, area, kind, issuedAt, answerDeadlineAt := issuedQuestionToDBFields(issuedQuestion)
	row := s.pool.QueryRow(ctx,
		`UPDATE sessions
		 SET flow_step = 'readiness',
		     expected_turn_id = $2,
		     active_question_text_es = $3,
		     active_question_text_en = $4,
		     active_question_area = $5,
		     active_question_kind = $6,
		     active_question_issued_at = $7,
		     active_answer_deadline_at = $8
		 WHERE session_code = $1
		 RETURNING flow_step,
		           COALESCE(expected_turn_id, ''),
		           display_question_number,
		           active_question_text_es,
		           active_question_text_en,
		           active_question_area,
		           active_question_kind,
		           active_question_issued_at,
		           active_answer_deadline_at`,
		sessionCode,
		issuedQuestion.Question.TurnID,
		textEs,
		textEn,
		area,
		kind,
		issuedAt,
		answerDeadlineAt,
	)

	var step string
	var expectedTurnID string
	var questionNumber int
	var persistedTextEs pgtype.Text
	var persistedTextEn pgtype.Text
	var persistedArea pgtype.Text
	var persistedKind pgtype.Text
	var persistedIssuedAt pgtype.Timestamptz
	var persistedAnswerDeadlineAt pgtype.Timestamptz
	if err := row.Scan(
		&step,
		&expectedTurnID,
		&questionNumber,
		&persistedTextEs,
		&persistedTextEn,
		&persistedArea,
		&persistedKind,
		&persistedIssuedAt,
		&persistedAnswerDeadlineAt,
	); err != nil {
		return nil, fmt.Errorf("prepare readiness step: %w", err)
	}

	return &FlowState{
		Step:           FlowStep(step),
		ExpectedTurnID: expectedTurnID,
		QuestionNumber: questionNumber,
		ActiveQuestion: issuedQuestionFromDB(
			expectedTurnID,
			questionNumber,
			persistedTextEs,
			persistedTextEn,
			persistedArea,
			persistedKind,
			persistedIssuedAt,
			persistedAnswerDeadlineAt,
		),
	}, nil
}

// AdvanceNonCriterionStep records an event and advances flow atomically.
func (s *PostgresStore) AdvanceNonCriterionStep(ctx context.Context, params AdvanceNonCriterionStepParams) (*FlowState, error) {
	if params.NextIssuedQuestion == nil {
		return nil, ErrInvalidFlow
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback on committed tx is a no-op

	var currentStep string
	var expectedTurnID string
	var questionNumber int
	lockRow := tx.QueryRow(ctx,
		`SELECT flow_step, COALESCE(expected_turn_id, ''), display_question_number
		 FROM sessions
		 WHERE session_code = $1
		 FOR UPDATE`,
		params.SessionCode,
	)
	if err := lockRow.Scan(&currentStep, &expectedTurnID, &questionNumber); err != nil {
		return nil, fmt.Errorf("lock flow state: %w", err)
	}

	if FlowStep(currentStep) != params.CurrentStep || expectedTurnID != params.ExpectedTurnID {
		return nil, ErrTurnConflict
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO interview_events (session_code, event_type, answer_text)
		 VALUES ($1, $2, $3)`,
		params.SessionCode,
		params.EventType,
		params.AnswerText,
	); err != nil {
		return nil, fmt.Errorf("insert interview event: %w", err)
	}

	textEs, textEn, area, kind, issuedAt, answerDeadlineAt := issuedQuestionToDBFields(params.NextIssuedQuestion)
	updateRow := tx.QueryRow(ctx,
		`UPDATE sessions
		 SET flow_step = $2,
		     expected_turn_id = $3,
		     display_question_number = $4,
		     active_question_text_es = $5,
		     active_question_text_en = $6,
		     active_question_area = $7,
		     active_question_kind = $8,
		     active_question_issued_at = $9,
		     active_answer_deadline_at = $10
		 WHERE session_code = $1
		 RETURNING flow_step,
		           COALESCE(expected_turn_id, ''),
		           display_question_number,
		           active_question_text_es,
		           active_question_text_en,
		           active_question_area,
		           active_question_kind,
		           active_question_issued_at,
		           active_answer_deadline_at`,
		params.SessionCode,
		string(params.NextStep),
		params.NextIssuedQuestion.Question.TurnID,
		params.NextIssuedQuestion.Question.QuestionNumber,
		textEs,
		textEn,
		area,
		kind,
		issuedAt,
		answerDeadlineAt,
	)

	var newStep string
	var newTurnID string
	var newQuestionNumber int
	var persistedTextEs pgtype.Text
	var persistedTextEn pgtype.Text
	var persistedArea pgtype.Text
	var persistedKind pgtype.Text
	var persistedIssuedAt pgtype.Timestamptz
	var persistedAnswerDeadlineAt pgtype.Timestamptz
	if err := updateRow.Scan(
		&newStep,
		&newTurnID,
		&newQuestionNumber,
		&persistedTextEs,
		&persistedTextEn,
		&persistedArea,
		&persistedKind,
		&persistedIssuedAt,
		&persistedAnswerDeadlineAt,
	); err != nil {
		return nil, fmt.Errorf("advance non-criterion flow: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	return &FlowState{
		Step:           FlowStep(newStep),
		ExpectedTurnID: newTurnID,
		QuestionNumber: newQuestionNumber,
		ActiveQuestion: issuedQuestionFromDB(
			newTurnID,
			newQuestionNumber,
			persistedTextEs,
			persistedTextEn,
			persistedArea,
			persistedKind,
			persistedIssuedAt,
			persistedAnswerDeadlineAt,
		),
	}, nil
}

// ProcessCriterionTurn persists one scored criterion answer and transition atomically.
func (s *PostgresStore) ProcessCriterionTurn(ctx context.Context, params ProcessCriterionTurnParams) (*ProcessCriterionTurnResult, error) {
	if params.Evaluation == nil {
		return nil, ErrInvalidFlow
	}
	if strings.TrimSpace(params.NextArea) != "" && params.NextIssuedQuestion == nil {
		return nil, ErrInvalidFlow
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback on committed tx is a no-op

	var currentStep string
	var expectedTurnID string
	lockRow := tx.QueryRow(ctx,
		`SELECT flow_step, COALESCE(expected_turn_id, '')
		 FROM sessions
		 WHERE session_code = $1
		 FOR UPDATE`,
		params.SessionCode,
	)
	if err := lockRow.Scan(&currentStep, &expectedTurnID); err != nil {
		return nil, fmt.Errorf("lock criterion flow: %w", err)
	}
	if FlowStep(currentStep) != FlowStepCriterion || expectedTurnID != params.ExpectedTurnID {
		return nil, ErrTurnConflict
	}

	var transcriptEs, transcriptEn string
	if strings.EqualFold(strings.TrimSpace(params.PreferredLanguage), "en") {
		transcriptEn = params.AnswerText
	} else {
		transcriptEs = params.AnswerText
	}

	evalJSON, _ := json.Marshal(params.Evaluation)
	if _, err := tx.Exec(ctx,
		`INSERT INTO answers (session_code, area, question_text, transcript_es, transcript_en, ai_evaluation, sufficiency)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		params.SessionCode,
		params.CurrentArea,
		params.QuestionText,
		nullIfEmpty(transcriptEs),
		nullIfEmpty(transcriptEn),
		evalJSON,
		nullIfEmpty(string(params.Evaluation.CurrentCriterion.Status)),
	); err != nil {
		return nil, fmt.Errorf("insert answer: %w", err)
	}

	// Accumulate lapsed seconds: min(now - question issued, answer time limit).
	if _, err := tx.Exec(ctx,
		`UPDATE sessions
		 SET interview_lapsed_seconds = interview_lapsed_seconds + LEAST(
		     EXTRACT(EPOCH FROM (now() - active_question_issued_at))::int,
		     $2
		 ),
		 interview_lapsed_updated_at = now()
		 WHERE session_code = $1 AND active_question_issued_at IS NOT NULL`,
		params.SessionCode,
		params.AnswerTimeLimitSeconds,
	); err != nil {
		return nil, fmt.Errorf("update lapsed seconds: %w", err)
	}

	var newCount int
	countRow := tx.QueryRow(ctx,
		`UPDATE question_areas
		 SET questions_count = questions_count + 1
		 WHERE session_code = $1 AND area = $2
		 RETURNING questions_count`,
		params.SessionCode,
		params.CurrentArea,
	)
	if err := countRow.Scan(&newCount); err != nil {
		return nil, fmt.Errorf("increment area questions: %w", err)
	}

	switch params.Decision.MarkCurrentAs {
	case AreaStatusComplete:
		if _, err := tx.Exec(ctx,
			`UPDATE question_areas
			 SET status = 'complete', area_ended_at = now()
			 WHERE session_code = $1 AND area = $2`,
			params.SessionCode,
			params.CurrentArea,
		); err != nil {
			return nil, fmt.Errorf("complete area: %w", err)
		}
	case AreaStatusInsufficient:
		if _, err := tx.Exec(ctx,
			`UPDATE question_areas
			 SET status = 'insufficient', area_ended_at = now()
			 WHERE session_code = $1 AND area = $2`,
			params.SessionCode,
			params.CurrentArea,
		); err != nil {
			return nil, fmt.Errorf("mark area insufficient: %w", err)
		}
	}

	for _, flag := range params.PreAddressed {
		if strings.TrimSpace(flag.Slug) == "" {
			continue
		}
		if _, err := tx.Exec(ctx,
			`UPDATE question_areas
			 SET status = 'pre_addressed', pre_addressed_evidence = $3
			 WHERE session_code = $1
			   AND LOWER(area) = LOWER($2)
			   AND status = 'pending'`,
			params.SessionCode,
			flag.Slug,
			nullIfEmpty(flag.Evidence),
		); err != nil {
			return nil, fmt.Errorf("mark pre_addressed %s: %w", flag.Slug, err)
		}
	}

	if strings.TrimSpace(params.NextArea) != "" && params.Decision.Action == CriterionTurnActionNext {
		if _, err := tx.Exec(ctx,
			`UPDATE question_areas
			 SET status = 'in_progress', area_started_at = now()
			 WHERE session_code = $1
			   AND area = $2
			   AND status IN ('pending', 'pre_addressed')`,
			params.SessionCode,
			params.NextArea,
		); err != nil {
			return nil, fmt.Errorf("set next area in_progress: %w", err)
		}
	}

	textEs, textEn, area, kind, issuedAt, answerDeadlineAt := issuedQuestionToDBFields(params.NextIssuedQuestion)
	updateStateRow := tx.QueryRow(ctx,
		`UPDATE sessions
		 SET flow_step = CASE WHEN $2 = '' THEN 'done' ELSE 'criterion' END,
		     expected_turn_id = CASE WHEN $2 = '' THEN NULL ELSE $3 END,
		     display_question_number = CASE WHEN $2 = '' THEN display_question_number + 1 ELSE $4 END,
		     active_question_text_es = $5,
		     active_question_text_en = $6,
		     active_question_area = $7,
		     active_question_kind = $8,
		     active_question_issued_at = $9,
		     active_answer_deadline_at = $10
		 WHERE session_code = $1
		 RETURNING display_question_number`,
		params.SessionCode,
		params.NextArea,
		nullIfEmpty(func() string {
			if params.NextIssuedQuestion == nil {
				return ""
			}
			return params.NextIssuedQuestion.Question.TurnID
		}()),
		func() int {
			if params.NextIssuedQuestion == nil {
				return 0
			}
			return params.NextIssuedQuestion.Question.QuestionNumber
		}(),
		textEs,
		textEn,
		area,
		kind,
		issuedAt,
		answerDeadlineAt,
	)
	var persistedQuestionNumber int
	if err := updateStateRow.Scan(&persistedQuestionNumber); err != nil {
		return nil, fmt.Errorf("advance flow state after criterion: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit criterion turn: %w", err)
	}

	return &ProcessCriterionTurnResult{
		NewCount: newCount,
	}, nil
}

// MarkFlowDone marks the flow pointer as done and clears expected turn.
func (s *PostgresStore) MarkFlowDone(ctx context.Context, sessionCode string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE sessions
		 SET flow_step = 'done',
		     expected_turn_id = NULL,
		     active_question_text_es = NULL,
		     active_question_text_en = NULL,
		     active_question_area = NULL,
		     active_question_kind = NULL,
		     active_question_issued_at = NULL,
		     active_answer_deadline_at = NULL
		 WHERE session_code = $1`,
		sessionCode,
	)
	if err != nil {
		return fmt.Errorf("mark flow done: %w", err)
	}
	return nil
}

func nullIfEmpty(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}
