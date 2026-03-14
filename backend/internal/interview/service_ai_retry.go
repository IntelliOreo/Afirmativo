package interview

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

var aiRetryBackoffs = []time.Duration{
	3 * time.Second,
	7 * time.Second,
}

type aiRetryFailureRecorder interface {
	RecordFailure(ctx context.Context, reason string, incrementAttempts bool)
}

type aiRetryFailureRecorderFunc func(ctx context.Context, reason string, incrementAttempts bool)

func (f aiRetryFailureRecorderFunc) RecordFailure(ctx context.Context, reason string, incrementAttempts bool) {
	f(ctx, reason, incrementAttempts)
}

func formatAIFailureReason(attempt int, err error) string {
	var providerFailure *AIProviderFailure
	if errors.As(err, &providerFailure) {
		return fmt.Sprintf(
			"attempt=%d http_status=%d error_type=%s error_message=%s payload_excerpt=%s",
			attempt,
			providerFailure.HTTPStatus,
			strings.TrimSpace(providerFailure.ErrorType),
			truncateWithPrefix(providerFailure.ErrorMessage, aiFailurePayloadExcerptMaxChars),
			providerFailure.PayloadExcerpt,
		)
	}
	return fmt.Sprintf(
		"attempt=%d http_status=0 error_type=unstructured_error error_message=%s",
		attempt,
		truncateWithPrefix(err.Error(), aiFailurePayloadExcerptMaxChars),
	)
}

func (s *Service) callAIWithRetry(ctx context.Context, turnCtx *AITurnContext, failureRecorder aiRetryFailureRecorder) (*AIResponse, error) {
	tracer := otel.Tracer("afirmativo-interview")
	ctx, span := tracer.Start(ctx, "ai.generate_turn")
	defer span.End()
	span.SetAttributes(
		attribute.String("ai.area", turnCtx.CurrentAreaSlug),
	)

	maxAttempts := len(aiRetryBackoffs) + 1
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		aiResult, err := s.aiClient.GenerateTurn(ctx, turnCtx)
		if err == nil {
			return aiResult, nil
		}

		reason := formatAIFailureReason(attempt, err)
		retrying := attempt < maxAttempts
		if failureRecorder != nil {
			failureRecorder.RecordFailure(ctx, reason, retrying)
		}

		if !retrying {
			return nil, fmt.Errorf("%w: %s", ErrAIRetryExhausted, reason)
		}

		delay := aiRetryBackoffs[attempt-1]
		slog.Warn("AI call failed; retrying",
			"area", turnCtx.CurrentAreaSlug,
			"attempt", attempt,
			"max_attempts", maxAttempts,
			"retry_in", delay,
			"error", err,
		)

		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, fmt.Errorf("AI retry aborted: %w", ctx.Err())
		case <-timer.C:
		}
	}

	return nil, fmt.Errorf("%w: exhausted", ErrAIRetryExhausted)
}
