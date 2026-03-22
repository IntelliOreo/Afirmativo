package interview

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/afirmativo/backend/internal/config"
	"github.com/afirmativo/backend/internal/shared"
	"github.com/afirmativo/backend/internal/vertexai"
)

// VertexInterviewAIClientConfig holds configuration for the Vertex interview AI client.
type VertexInterviewAIClientConfig struct {
	Model                   string
	MaxTokens               int
	AllowSensitiveDebugLogs bool
	SystemPrompt            string
	PromptLastQuestion      string
	PromptClosing           string
	PromptOpeningTurn       string
	LastQuestionSeconds     int
	ClosingSeconds          int
	MidpointAreaIndex       int
	AreaConfigs             []config.AreaConfig
	VertexClient            *vertexai.Client
}

// VertexInterviewAIClient implements InterviewAIClient using Vertex AI.
type VertexInterviewAIClient struct {
	model                   string
	maxTokens               int
	allowSensitiveDebugLogs bool
	openingTurnPrompt       string
	vertexClient            *vertexai.Client
	promptComposer          promptComposer
}

// NewVertexInterviewAIClient creates a new Vertex-backed interview AI client.
func NewVertexInterviewAIClient(cfg VertexInterviewAIClientConfig) *VertexInterviewAIClient {
	return &VertexInterviewAIClient{
		model:                   cfg.Model,
		maxTokens:               cfg.MaxTokens,
		allowSensitiveDebugLogs: cfg.AllowSensitiveDebugLogs,
		openingTurnPrompt:       cfg.PromptOpeningTurn,
		vertexClient:            cfg.VertexClient,
		promptComposer: promptComposer{
			systemPrompt:      cfg.SystemPrompt,
			promptLastQ:       cfg.PromptLastQuestion,
			promptClosing:     cfg.PromptClosing,
			lastQSeconds:      cfg.LastQuestionSeconds,
			closingSeconds:    cfg.ClosingSeconds,
			midpointAreaIndex: cfg.MidpointAreaIndex,
		},
	}
}

// GenerateTurn sends the interview turn context to Vertex and parses structured JSON output.
func (c *VertexInterviewAIClient) GenerateTurn(ctx context.Context, turnCtx *AITurnContext) (*AIResponse, error) {
	priorityPrompt := c.promptComposer.composePriority(turnCtx)
	contents, turnUserMessage := buildVertexInterviewContents(turnCtx, priorityPrompt, c.openingTurnPrompt)
	systemInstruction := vertexai.NewTextContent("", c.promptComposer.composeSystem())

	requestBody := vertexai.GenerateContentRequest{
		SystemInstruction: &systemInstruction,
		Contents: contents,
		GenerationConfig: vertexai.GenerationConfig{
			MaxOutputTokens:  c.maxTokens,
			ResponseMIMEType: "application/json",
			ResponseSchema:   buildVertexInterviewResponseSchema(),
		},
	}

	slog.Debug("calling AI API",
		"provider", "vertex",
		"phase", "interview",
		"area", turnCtx.CurrentAreaSlug,
		"model", c.model,
		"cache_mode", "implicit_only",
		"sensitive_debug_logs_enabled", c.allowSensitiveDebugLogs,
	)
	if c.allowSensitiveDebugLogs {
		shared.DebugTextBlock("Vertex request system prompt", c.promptComposer.composeSystem())
		shared.DebugTextBlock("Vertex request turn user message", turnUserMessage)
		shared.DebugJSON("Vertex request body", requestBody)
	}

	resp, err := c.vertexClient.GenerateContent(ctx, c.model, requestBody)
	if err != nil {
		return nil, toInterviewAIProviderFailure(err)
	}
	if c.allowSensitiveDebugLogs {
		shared.DebugJSON("Vertex raw response", resp)
	}

	jsonStr, err := vertexResponseText(resp)
	if err != nil {
		return nil, err
	}
	result, err := parseAIResponseJSON(jsonStr)
	if err != nil {
		return nil, err
	}
	if turnCtx.IsOpeningTurn {
		result.Evaluation = nil
	}

	slog.Info("AI API usage",
		"phase", "interview",
		"area", turnCtx.CurrentAreaSlug,
		"is_opening_turn", turnCtx.IsOpeningTurn,
		"provider", "vertex",
		"model", c.model,
		"model_version", resp.ModelVersion,
		"prompt_tokens", resp.UsageMetadata.PromptTokenCount,
		"candidate_tokens", resp.UsageMetadata.CandidatesTokenCount,
		"total_tokens", resp.UsageMetadata.TotalTokenCount,
		"cached_content_tokens", resp.UsageMetadata.CachedContentTokenCount,
		"cache_mode", "implicit_only",
	)

	return result, nil
}

func buildVertexInterviewContents(turnCtx *AITurnContext, priorityPrompt, openingTurnPrompt string) ([]vertexai.Content, string) {
	turnUserMessage := buildTurnUserMessage(turnCtx, priorityPrompt, openingTurnPrompt)
	contents := make([]vertexai.Content, 0, len(turnCtx.HistoryTurns)*2+2)
	for _, turn := range turnCtx.HistoryTurns {
		contents = append(contents,
			vertexai.NewTextContent("model", turn.QuestionText),
			vertexai.NewTextContent("user", turn.AnswerText),
		)
	}
	if !turnCtx.IsOpeningTurn {
		contents = append(contents, vertexai.NewTextContent("model", turnCtx.CurrentQuestionText))
	}
	contents = append(contents, vertexai.NewTextContent("user", turnUserMessage))
	return contents, turnUserMessage
}

func buildVertexInterviewResponseSchema() map[string]any {
	outputSchema := buildOutputSchema()
	format, _ := outputSchema["format"].(map[string]interface{})
	schema, _ := format["schema"].(map[string]interface{})
	return vertexai.NormalizeResponseSchema(schema)
}

func vertexResponseText(resp *vertexai.GenerateContentResponse) (string, error) {
	if len(resp.Candidates) == 0 {
		if reason := strings.TrimSpace(resp.PromptFeedback.BlockReason); reason != "" {
			return "", &AIProviderFailure{
				HTTPStatus:   200,
				ErrorType:    "prompt_blocked",
				ErrorMessage: reason,
			}
		}
		return "", &AIProviderFailure{
			HTTPStatus:   200,
			ErrorType:    "empty_candidates",
			ErrorMessage: "empty candidates in Vertex response",
		}
	}

	first := resp.Candidates[0]
	switch strings.TrimSpace(first.FinishReason) {
	case "", "STOP":
	case "MAX_TOKENS":
		return "", &AIProviderFailure{
			HTTPStatus:   200,
			ErrorType:    "stop_reason_max_tokens",
			ErrorMessage: "response truncated due to max tokens",
		}
	case "SAFETY", "PROHIBITED_CONTENT", "SPII":
		return "", &AIProviderFailure{
			HTTPStatus:   200,
			ErrorType:    "stop_reason_blocked",
			ErrorMessage: first.FinishReason,
		}
	default:
		return "", &AIProviderFailure{
			HTTPStatus:   200,
			ErrorType:    "stop_reason_" + strings.ToLower(first.FinishReason),
			ErrorMessage: first.FinishReason,
		}
	}

	if len(first.Content.Parts) == 0 || strings.TrimSpace(first.Content.Parts[0].Text) == "" {
		return "", &AIProviderFailure{
			HTTPStatus:   200,
			ErrorType:    "empty_content",
			ErrorMessage: "empty text parts in Vertex response",
		}
	}
	return first.Content.Parts[0].Text, nil
}

func toInterviewAIProviderFailure(err error) error {
	var vertexErr *vertexai.APIError
	if !errors.As(err, &vertexErr) {
		return &AIProviderFailure{
			ErrorType:    "transport_error",
			ErrorMessage: err.Error(),
		}
	}
	return &AIProviderFailure{
		HTTPStatus:     vertexErr.StatusCode,
		ErrorType:      strings.TrimSpace(vertexErr.Status),
		ErrorMessage:   strings.TrimSpace(vertexErr.Message),
		PayloadExcerpt: truncateWithPrefix(vertexErr.RawBody, aiFailurePayloadExcerptMaxChars),
	}
}
