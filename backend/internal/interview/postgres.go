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

// GetFlowState returns the persisted interview flow pointer for a session.
func (s *PostgresStore) GetFlowState(ctx context.Context, sessionCode string) (*FlowState, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT flow_step, COALESCE(expected_turn_id, ''), display_question_number
		 FROM sessions
		 WHERE session_code = $1`,
		sessionCode,
	)

	var step string
	var turnID string
	var questionNumber int
	if err := row.Scan(&step, &turnID, &questionNumber); err != nil {
		return nil, fmt.Errorf("get flow state: %w", err)
	}

	return &FlowState{
		Step:           FlowStep(step),
		ExpectedTurnID: turnID,
		QuestionNumber: questionNumber,
	}, nil
}

// PrepareDisclaimerStep forces the flow pointer to disclaimer and sets turn id.
func (s *PostgresStore) PrepareDisclaimerStep(ctx context.Context, sessionCode, turnID string) (*FlowState, error) {
	row := s.pool.QueryRow(ctx,
		`UPDATE sessions
		 SET flow_step = 'disclaimer',
		     expected_turn_id = $2
		 WHERE session_code = $1
		 RETURNING flow_step, COALESCE(expected_turn_id, ''), display_question_number`,
		sessionCode,
		turnID,
	)

	var step string
	var expectedTurnID string
	var questionNumber int
	if err := row.Scan(&step, &expectedTurnID, &questionNumber); err != nil {
		return nil, fmt.Errorf("prepare disclaimer step: %w", err)
	}

	return &FlowState{
		Step:           FlowStep(step),
		ExpectedTurnID: expectedTurnID,
		QuestionNumber: questionNumber,
	}, nil
}

// AdvanceNonCriterionStep records an event and advances flow atomically.
func (s *PostgresStore) AdvanceNonCriterionStep(ctx context.Context, params AdvanceNonCriterionStepParams) (*FlowState, error) {
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

	updateRow := tx.QueryRow(ctx,
		`UPDATE sessions
		 SET flow_step = $2,
		     expected_turn_id = $3
		 WHERE session_code = $1
		 RETURNING flow_step, COALESCE(expected_turn_id, ''), display_question_number`,
		params.SessionCode,
		string(params.NextStep),
		params.NextTurnID,
	)

	var newStep string
	var newTurnID string
	var newQuestionNumber int
	if err := updateRow.Scan(&newStep, &newTurnID, &newQuestionNumber); err != nil {
		return nil, fmt.Errorf("advance non-criterion flow: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	return &FlowState{
		Step:           FlowStep(newStep),
		ExpectedTurnID: newTurnID,
		QuestionNumber: newQuestionNumber,
	}, nil
}

// ProcessCriterionTurn persists one scored criterion answer and transition atomically.
func (s *PostgresStore) ProcessCriterionTurn(ctx context.Context, params ProcessCriterionTurnParams) (*ProcessCriterionTurnResult, error) {
	if params.Evaluation == nil {
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
		nullIfEmpty(params.Evaluation.CurrentCriterion.Status),
	); err != nil {
		return nil, fmt.Errorf("insert answer: %w", err)
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

	action := "stay"
	if params.Evaluation.CurrentCriterion.Status == "sufficient" {
		if _, err := tx.Exec(ctx,
			`UPDATE question_areas
			 SET status = 'complete', area_ended_at = now()
			 WHERE session_code = $1 AND area = $2`,
			params.SessionCode,
			params.CurrentArea,
		); err != nil {
			return nil, fmt.Errorf("complete area: %w", err)
		}
		action = "next"
	} else if newCount >= params.MaxQuestionsPerArea || params.Evaluation.CurrentCriterion.Recommendation == "move_on" {
		if _, err := tx.Exec(ctx,
			`UPDATE question_areas
			 SET status = 'insufficient', area_ended_at = now()
			 WHERE session_code = $1 AND area = $2`,
			params.SessionCode,
			params.CurrentArea,
		); err != nil {
			return nil, fmt.Errorf("mark area insufficient: %w", err)
		}
		action = "next"
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

	nextArea := params.CurrentArea
	if action == "next" {
		rows, err := tx.Query(ctx,
			`SELECT area, status
			 FROM question_areas
			 WHERE session_code = $1`,
			params.SessionCode,
		)
		if err != nil {
			return nil, fmt.Errorf("list areas for transition: %w", err)
		}

		statusByArea := make(map[string]string)
		for rows.Next() {
			var area string
			var status string
			if err := rows.Scan(&area, &status); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan area status: %w", err)
			}
			statusByArea[area] = status
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, fmt.Errorf("iterate area statuses: %w", err)
		}
		rows.Close()

		nextArea = ""
		for _, slug := range params.OrderedAreaSlugs {
			status, ok := statusByArea[slug]
			if !ok {
				continue
			}
			if status == string(AreaStatusPending) || status == string(AreaStatusPreAddressed) {
				nextArea = slug
				break
			}
		}

		if nextArea != "" {
			if _, err := tx.Exec(ctx,
				`UPDATE question_areas
				 SET status = 'in_progress', area_started_at = now()
				 WHERE session_code = $1
				   AND area = $2
				   AND status IN ('pending', 'pre_addressed')`,
				params.SessionCode,
				nextArea,
			); err != nil {
				return nil, fmt.Errorf("set next area in_progress: %w", err)
			}
		}
	}

	updateStateRow := tx.QueryRow(ctx,
		`UPDATE sessions
		 SET flow_step = CASE WHEN $2 = '' THEN 'done' ELSE 'criterion' END,
		     expected_turn_id = CASE WHEN $2 = '' THEN NULL ELSE $3 END,
		     display_question_number = display_question_number + 1
		 WHERE session_code = $1
		 RETURNING display_question_number`,
		params.SessionCode,
		nextArea,
		params.NextTurnID,
	)
	var questionNumber int
	if err := updateStateRow.Scan(&questionNumber); err != nil {
		return nil, fmt.Errorf("advance flow state after criterion: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit criterion turn: %w", err)
	}

	return &ProcessCriterionTurnResult{
		Action:         action,
		NextArea:       nextArea,
		QuestionNumber: questionNumber,
		NewCount:       newCount,
	}, nil
}

// MarkFlowDone marks the flow pointer as done and clears expected turn.
func (s *PostgresStore) MarkFlowDone(ctx context.Context, sessionCode string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE sessions
		 SET flow_step = 'done',
		     expected_turn_id = NULL
		 WHERE session_code = $1`,
		sessionCode,
	)
	if err != nil {
		return fmt.Errorf("mark flow done: %w", err)
	}
	return nil
}

// UpsertAnswerJob creates or returns an existing async answer job by idempotency key.
func (s *PostgresStore) UpsertAnswerJob(ctx context.Context, params UpsertAnswerJobParams) (*AnswerJob, error) {
	row := s.pool.QueryRow(ctx,
		`INSERT INTO interview_answer_jobs (
		     session_code,
		     client_request_id,
		     turn_id,
		     question_text,
		     answer_text,
		     status
		 )
		 VALUES ($1, $2, $3, $4, $5, 'queued')
		 ON CONFLICT (session_code, client_request_id)
		 DO UPDATE SET updated_at = now()
		 RETURNING id, session_code, client_request_id, turn_id, question_text, answer_text, status,
		           result_payload, error_code, error_message, attempts, started_at, completed_at, created_at, updated_at`,
		params.SessionCode,
		params.ClientRequestID,
		params.TurnID,
		nullIfEmpty(params.QuestionText),
		params.AnswerText,
	)

	job, err := scanAnswerJob(row)
	if err != nil {
		return nil, fmt.Errorf("upsert answer job: %w", err)
	}
	return job, nil
}

// ClaimQueuedAnswerJob moves a queued job to running atomically.
// Returns nil,nil when the job is already claimed or in terminal state.
func (s *PostgresStore) ClaimQueuedAnswerJob(ctx context.Context, jobID string) (*AnswerJob, error) {
	row := s.pool.QueryRow(ctx,
		`UPDATE interview_answer_jobs
		 SET status = 'running',
		     attempts = attempts + 1,
		     started_at = COALESCE(started_at, now()),
		     updated_at = now()
		 WHERE id = $1::uuid
		   AND status = 'queued'
		 RETURNING id, session_code, client_request_id, turn_id, question_text, answer_text, status,
		           result_payload, error_code, error_message, attempts, started_at, completed_at, created_at, updated_at`,
		jobID,
	)

	job, err := scanAnswerJob(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("claim answer job: %w", err)
	}
	return job, nil
}

// GetAnswerJob returns a polling job by session and job ID.
func (s *PostgresStore) GetAnswerJob(ctx context.Context, sessionCode, jobID string) (*AnswerJob, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, session_code, client_request_id, turn_id, question_text, answer_text, status,
		        result_payload, error_code, error_message, attempts, started_at, completed_at, created_at, updated_at
		   FROM interview_answer_jobs
		  WHERE session_code = $1
		    AND id = $2::uuid`,
		sessionCode,
		jobID,
	)

	job, err := scanAnswerJob(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAsyncJobNotFound
		}
		return nil, fmt.Errorf("get answer job: %w", err)
	}
	return job, nil
}

// MarkAnswerJobSucceeded stores terminal success state and result payload.
func (s *PostgresStore) MarkAnswerJobSucceeded(ctx context.Context, jobID string, resultPayload []byte) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE interview_answer_jobs
		 SET status = 'succeeded',
		     result_payload = $2,
		     error_code = NULL,
		     error_message = NULL,
		     completed_at = now(),
		     updated_at = now()
		 WHERE id = $1::uuid`,
		jobID,
		resultPayload,
	)
	if err != nil {
		return fmt.Errorf("mark answer job succeeded: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrAsyncJobNotFound
	}
	return nil
}

// MarkAnswerJobFailed stores terminal failure/conflict state.
func (s *PostgresStore) MarkAnswerJobFailed(ctx context.Context, params MarkAnswerJobFailedParams) error {
	status := params.Status
	if status != AsyncAnswerJobFailed && status != AsyncAnswerJobConflict && status != AsyncAnswerJobCanceled {
		status = AsyncAnswerJobFailed
	}

	tag, err := s.pool.Exec(ctx,
		`UPDATE interview_answer_jobs
		 SET status = $2,
		     error_code = $3,
		     error_message = $4,
		     completed_at = now(),
		     updated_at = now()
		 WHERE id = $1::uuid`,
		params.JobID,
		string(status),
		nullIfEmpty(params.ErrorCode),
		nullIfEmpty(params.ErrorMessage),
	)
	if err != nil {
		return fmt.Errorf("mark answer job failed: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrAsyncJobNotFound
	}
	return nil
}

func scanAnswerJob(row pgx.Row) (*AnswerJob, error) {
	var id pgtype.UUID
	var sessionCode string
	var clientRequestID string
	var turnID string
	var questionText pgtype.Text
	var answerText string
	var status string
	var resultPayload []byte
	var errorCode pgtype.Text
	var errorMessage pgtype.Text
	var attempts int32
	var startedAt pgtype.Timestamptz
	var completedAt pgtype.Timestamptz
	var createdAt pgtype.Timestamptz
	var updatedAt pgtype.Timestamptz

	if err := row.Scan(
		&id,
		&sessionCode,
		&clientRequestID,
		&turnID,
		&questionText,
		&answerText,
		&status,
		&resultPayload,
		&errorCode,
		&errorMessage,
		&attempts,
		&startedAt,
		&completedAt,
		&createdAt,
		&updatedAt,
	); err != nil {
		return nil, err
	}

	job := &AnswerJob{
		ID:              uuidToString(id),
		SessionCode:     sessionCode,
		ClientRequestID: clientRequestID,
		TurnID:          turnID,
		AnswerText:      answerText,
		Status:          AsyncAnswerJobStatus(status),
		ResultPayload:   resultPayload,
		Attempts:        int(attempts),
		CreatedAt:       createdAt.Time,
		UpdatedAt:       updatedAt.Time,
	}
	if questionText.Valid {
		job.QuestionText = questionText.String
	}
	if errorCode.Valid {
		job.ErrorCode = errorCode.String
	}
	if errorMessage.Valid {
		job.ErrorMessage = errorMessage.String
	}
	if startedAt.Valid {
		t := startedAt.Time
		job.StartedAt = &t
	}
	if completedAt.Valid {
		t := completedAt.Time
		job.CompletedAt = &t
	}

	return job, nil
}

func nullIfEmpty(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}
