package interview

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/afirmativo/backend/internal/session"
)

type turnSnapshot struct {
	session           *session.Session
	flowState         *FlowState
	areas             []QuestionArea
	currentArea       *QuestionArea
	preferredLanguage string
	answerText        string
	questionText      string
	turnID            string
	submissionTime    time.Time
	timeRemainingS    int
	failureRecorder   aiRetryFailureRecorder
}

func (s *Service) buildTurnSnapshot(
	ctx context.Context,
	sessionCode, answerText, questionText, turnID string,
	submissionTime time.Time,
	failureRecorder aiRetryFailureRecorder,
) (*turnSnapshot, error) {
	normalizedSubmissionTime := s.normalizeSubmissionTime(submissionTime)

	dbCtx, dbCancel := context.WithTimeout(ctx, s.dbTimeout)
	sess, err := s.sessionGetter.GetSessionByCode(dbCtx, sessionCode)
	dbCancel()
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	if s.nowFn().After(sess.ExpiresAt) {
		slog.Debug("submit answer rejected: session expired", "session_code", sessionCode)
		return nil, session.ErrSessionExpired
	}

	dbCtx, dbCancel = context.WithTimeout(ctx, s.dbTimeout)
	flowState, err := s.stateStore.GetFlowState(dbCtx, sessionCode)
	dbCancel()
	if err != nil {
		return nil, fmt.Errorf("get flow state: %w", err)
	}

	areas, currentArea, err := s.refreshAreaState(ctx, sessionCode)
	if err != nil {
		return nil, fmt.Errorf("refresh area state: %w", err)
	}

	preferredLanguage := normalizePreferredLanguage(sess.PreferredLanguage)
	normalizedAnswerText := answerText
	if strings.TrimSpace(normalizedAnswerText) == "" {
		normalizedAnswerText = emptyAnswerPlaceholder(preferredLanguage)
		slog.Info("empty answer replaced with placeholder",
			"session_code", sessionCode,
			"preferred_language", preferredLanguage,
		)
	}

	return &turnSnapshot{
		session:           sess,
		flowState:         flowState,
		areas:             areas,
		currentArea:       currentArea,
		preferredLanguage: preferredLanguage,
		answerText:        normalizedAnswerText,
		questionText:      questionText,
		turnID:            turnID,
		submissionTime:    normalizedSubmissionTime,
		timeRemainingS:    s.calcEffectiveTimeRemaining(sess, flowState, normalizedSubmissionTime),
		failureRecorder:   failureRecorder,
	}, nil
}
