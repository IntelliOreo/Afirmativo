package interview

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// ProcessCriterionTurn persists one scored criterion answer and transition atomically.
func (s *PostgresStore) ProcessCriterionTurn(ctx context.Context, params ProcessCriterionTurnParams) (*ProcessCriterionTurnResult, error) {
	if err := validateProcessCriterionTurnParams(params); err != nil {
		return nil, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback on committed tx is a no-op

	if err := s.lockCriterionFlow(ctx, tx, params.SessionCode, params.ExpectedTurnID); err != nil {
		return nil, err
	}

	transcriptEs, transcriptEn := criterionAnswerTranscripts(params.PreferredLanguage, params.AnswerText)
	if err := s.insertCriterionAnswer(ctx, tx, params, transcriptEs, transcriptEn); err != nil {
		return nil, err
	}
	if err := s.updateCriterionLapsedSeconds(ctx, tx, params); err != nil {
		return nil, err
	}

	newCount, err := s.incrementCriterionAreaCount(ctx, tx, params.SessionCode, params.CurrentArea)
	if err != nil {
		return nil, err
	}
	if err := s.applyCriterionAreaTerminalStatus(ctx, tx, params.SessionCode, params.CurrentArea, params.Decision.MarkCurrentAs); err != nil {
		return nil, err
	}
	if err := s.applyCriterionPreAddressed(ctx, tx, params.SessionCode, params.PreAddressed); err != nil {
		return nil, err
	}
	if err := s.promoteCriterionNextArea(ctx, tx, params.SessionCode, params.NextArea, params.Decision.Action); err != nil {
		return nil, err
	}
	if err := s.persistCriterionFlowState(ctx, tx, params); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit criterion turn: %w", err)
	}

	return &ProcessCriterionTurnResult{NewCount: newCount}, nil
}

func validateProcessCriterionTurnParams(params ProcessCriterionTurnParams) error {
	if params.Evaluation == nil {
		return ErrInvalidFlow
	}
	if params.SubmissionTime.IsZero() {
		return ErrInvalidFlow
	}
	if strings.TrimSpace(params.NextArea) != "" && params.NextIssuedQuestion == nil {
		return ErrInvalidFlow
	}
	return nil
}

func (s *PostgresStore) lockCriterionFlow(ctx context.Context, tx pgx.Tx, sessionCode, expectedTurnID string) error {
	var currentStep string
	var persistedTurnID string
	lockRow := tx.QueryRow(ctx,
		`SELECT flow_step, COALESCE(expected_turn_id, '')
		 FROM sessions
		 WHERE session_code = $1
		 FOR UPDATE`,
		sessionCode,
	)
	if err := lockRow.Scan(&currentStep, &persistedTurnID); err != nil {
		return fmt.Errorf("lock criterion flow: %w", err)
	}
	if FlowStep(currentStep) != FlowStepCriterion || persistedTurnID != expectedTurnID {
		return ErrTurnConflict
	}
	return nil
}

func criterionAnswerTranscripts(preferredLanguage, answerText string) (string, string) {
	if strings.EqualFold(strings.TrimSpace(preferredLanguage), "en") {
		return "", answerText
	}
	return answerText, ""
}

func (s *PostgresStore) insertCriterionAnswer(ctx context.Context, tx pgx.Tx, params ProcessCriterionTurnParams, transcriptEs, transcriptEn string) error {
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
		return fmt.Errorf("insert answer: %w", err)
	}
	return nil
}

func (s *PostgresStore) updateCriterionLapsedSeconds(ctx context.Context, tx pgx.Tx, params ProcessCriterionTurnParams) error {
	// Accumulate lapsed seconds from the durable submit time, not worker/commit time.
	if _, err := tx.Exec(ctx,
		`UPDATE sessions
		 SET interview_lapsed_seconds = interview_lapsed_seconds + LEAST(
		     GREATEST(EXTRACT(EPOCH FROM ($2::timestamptz - active_question_issued_at))::int, 0),
		     $3
		 ),
		 interview_lapsed_updated_at = $2
		 WHERE session_code = $1 AND active_question_issued_at IS NOT NULL`,
		params.SessionCode,
		params.SubmissionTime.UTC(),
		params.AnswerTimeLimitSeconds,
	); err != nil {
		return fmt.Errorf("update lapsed seconds: %w", err)
	}
	return nil
}

func (s *PostgresStore) incrementCriterionAreaCount(ctx context.Context, tx pgx.Tx, sessionCode, currentArea string) (int, error) {
	var newCount int
	countRow := tx.QueryRow(ctx,
		`UPDATE question_areas
		 SET questions_count = questions_count + 1
		 WHERE session_code = $1 AND area = $2
		 RETURNING questions_count`,
		sessionCode,
		currentArea,
	)
	if err := countRow.Scan(&newCount); err != nil {
		return 0, fmt.Errorf("increment area questions: %w", err)
	}
	return newCount, nil
}

func (s *PostgresStore) applyCriterionAreaTerminalStatus(ctx context.Context, tx pgx.Tx, sessionCode, currentArea string, status AreaStatus) error {
	switch status {
	case AreaStatusComplete:
		if _, err := tx.Exec(ctx,
			`UPDATE question_areas
			 SET status = 'complete', area_ended_at = now()
			 WHERE session_code = $1 AND area = $2`,
			sessionCode,
			currentArea,
		); err != nil {
			return fmt.Errorf("complete area: %w", err)
		}
	case AreaStatusInsufficient:
		if _, err := tx.Exec(ctx,
			`UPDATE question_areas
			 SET status = 'insufficient', area_ended_at = now()
			 WHERE session_code = $1 AND area = $2`,
			sessionCode,
			currentArea,
		); err != nil {
			return fmt.Errorf("mark area insufficient: %w", err)
		}
	}
	return nil
}

func (s *PostgresStore) applyCriterionPreAddressed(ctx context.Context, tx pgx.Tx, sessionCode string, flags []PreAddressedArea) error {
	for _, flag := range flags {
		if strings.TrimSpace(flag.Slug) == "" {
			continue
		}
		if _, err := tx.Exec(ctx,
			`UPDATE question_areas
			 SET status = 'pre_addressed', pre_addressed_evidence = $3
			 WHERE session_code = $1
			   AND LOWER(area) = LOWER($2)
			   AND status = 'pending'`,
			sessionCode,
			flag.Slug,
			nullIfEmpty(flag.Evidence),
		); err != nil {
			return fmt.Errorf("mark pre_addressed %s: %w", flag.Slug, err)
		}
	}
	return nil
}

func (s *PostgresStore) promoteCriterionNextArea(ctx context.Context, tx pgx.Tx, sessionCode, nextArea string, action CriterionTurnAction) error {
	if strings.TrimSpace(nextArea) == "" || action != CriterionTurnActionNext {
		return nil
	}
	if _, err := tx.Exec(ctx,
		`UPDATE question_areas
		 SET status = 'in_progress', area_started_at = now()
		 WHERE session_code = $1
		   AND area = $2
		   AND status IN ('pending', 'pre_addressed')`,
		sessionCode,
		nextArea,
	); err != nil {
		return fmt.Errorf("set next area in_progress: %w", err)
	}
	return nil
}

func (s *PostgresStore) persistCriterionFlowState(ctx context.Context, tx pgx.Tx, params ProcessCriterionTurnParams) error {
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
		return fmt.Errorf("advance flow state after criterion: %w", err)
	}
	return nil
}
