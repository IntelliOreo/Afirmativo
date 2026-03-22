package interview

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type postgresFlowQueryRower interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type flowStateUpdateParams struct {
	sessionCode    string
	step           FlowStep
	expectedTurnID string
	questionNumber pgtype.Int4
	issuedQuestion *IssuedQuestion
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

	return scanFlowStateRow(row, "get flow state")
}

// PrepareDisclaimerStep forces the flow pointer to disclaimer and sets turn id.
func (s *PostgresStore) PrepareDisclaimerStep(ctx context.Context, sessionCode string, issuedQuestion *IssuedQuestion) (*FlowState, error) {
	return s.updateFlowState(ctx, s.pool, flowStateUpdateParams{
		sessionCode:    sessionCode,
		step:           FlowStepDisclaimer,
		expectedTurnID: issuedQuestion.Question.TurnID,
		issuedQuestion: issuedQuestion,
	}, "prepare disclaimer step")
}

// PrepareReadinessStep forces the flow pointer to readiness and sets turn id.
func (s *PostgresStore) PrepareReadinessStep(ctx context.Context, sessionCode string, issuedQuestion *IssuedQuestion) (*FlowState, error) {
	return s.updateFlowState(ctx, s.pool, flowStateUpdateParams{
		sessionCode:    sessionCode,
		step:           FlowStepReadiness,
		expectedTurnID: issuedQuestion.Question.TurnID,
		issuedQuestion: issuedQuestion,
	}, "prepare readiness step")
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
	lockRow := tx.QueryRow(ctx,
		`SELECT flow_step, COALESCE(expected_turn_id, ''), display_question_number
		 FROM sessions
		 WHERE session_code = $1
		 FOR UPDATE`,
		params.SessionCode,
	)
	var questionNumber int
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

	nextFlow, err := s.updateFlowState(ctx, tx, flowStateUpdateParams{
		sessionCode:    params.SessionCode,
		step:           params.NextStep,
		expectedTurnID: params.NextIssuedQuestion.Question.TurnID,
		questionNumber: flowQuestionNumber(params.NextIssuedQuestion.Question.QuestionNumber),
		issuedQuestion: params.NextIssuedQuestion,
	}, "advance non-criterion flow")
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	return nextFlow, nil
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

func (s *PostgresStore) updateFlowState(ctx context.Context, db postgresFlowQueryRower, params flowStateUpdateParams, op string) (*FlowState, error) {
	textEs, textEn, area, kind, issuedAt, answerDeadlineAt := issuedQuestionToDBFields(params.issuedQuestion)
	row := db.QueryRow(ctx,
		`UPDATE sessions
		 SET flow_step = $2,
		     expected_turn_id = $3,
		     display_question_number = COALESCE($4, display_question_number),
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
		params.sessionCode,
		string(params.step),
		params.expectedTurnID,
		params.questionNumber,
		textEs,
		textEn,
		area,
		kind,
		issuedAt,
		answerDeadlineAt,
	)

	return scanFlowStateRow(row, op)
}

func scanFlowStateRow(row pgx.Row, op string) (*FlowState, error) {
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
		return nil, fmt.Errorf("%s: %w", op, err)
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

func flowQuestionNumber(questionNumber int) pgtype.Int4 {
	return pgtype.Int4{Int32: int32(questionNumber), Valid: true}
}
