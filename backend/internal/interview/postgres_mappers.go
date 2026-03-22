package interview

import (
	"fmt"
	"strings"

	"github.com/afirmativo/backend/internal/sqlgen"
	"github.com/jackc/pgx/v5/pgtype"
)

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

func nullIfEmpty(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}
