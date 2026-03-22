package interview

import (
	"encoding/json"
	"fmt"
)

// SubmitAnswerAsyncResult is returned when an async answer job is accepted.
type SubmitAnswerAsyncResult struct {
	JobID           string
	ClientRequestID string
	Status          AsyncAnswerJobStatus
}

// AnswerJobStatusResult is returned by polling for async answer job state.
type AnswerJobStatusResult struct {
	JobID                        string
	ClientRequestID              string
	Status                       AsyncAnswerJobStatus
	Done                         bool
	NextQuestion                 *Question
	TimerRemainingS              int
	AnswerSubmitWindowRemainingS int
	ErrorCode                    string
	ErrorMessage                 string
}

type answerJobPayload struct {
	Done                         bool                    `json:"done"`
	NextQuestion                 *answerJobQuestionShape `json:"next_question"`
	TimerRemainingS              int                     `json:"timer_remaining_s"`
	AnswerSubmitWindowRemainingS int                     `json:"answer_submit_window_remaining_s"`
}

type answerJobQuestionShape struct {
	TextEs         string `json:"text_es"`
	TextEn         string `json:"text_en"`
	Area           string `json:"area"`
	Kind           string `json:"kind"`
	TurnID         string `json:"turn_id"`
	QuestionNumber int    `json:"question_number"`
	TotalQuestions int    `json:"total_questions"`
}

func encodeAnswerJobPayload(result *AnswerResult) ([]byte, error) {
	return json.Marshal(toAnswerJobPayload(result))
}

func decodeAnswerJobPayload(resultPayload []byte) (*answerJobPayload, error) {
	var payload answerJobPayload
	if err := json.Unmarshal(resultPayload, &payload); err != nil {
		return nil, fmt.Errorf("decode answer job payload: %w", err)
	}
	return &payload, nil
}

func toAnswerJobPayload(result *AnswerResult) *answerJobPayload {
	payload := &answerJobPayload{
		Done:                         result.Done,
		TimerRemainingS:              result.TimerRemainingS,
		AnswerSubmitWindowRemainingS: result.AnswerSubmitWindowRemainingS,
	}
	if result.NextQuestion != nil {
		payload.NextQuestion = &answerJobQuestionShape{
			TextEs:         result.NextQuestion.TextEs,
			TextEn:         result.NextQuestion.TextEn,
			Area:           result.NextQuestion.Area,
			Kind:           string(result.NextQuestion.Kind),
			TurnID:         result.NextQuestion.TurnID,
			QuestionNumber: result.NextQuestion.QuestionNumber,
			TotalQuestions: result.NextQuestion.TotalQuestions,
		}
	}
	return payload
}
