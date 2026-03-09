// HTTP client that calls the Claude API (or mock server) to evaluate answers
// and generate the next interview question.
package interview

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/afirmativo/backend/internal/config"
	"github.com/afirmativo/backend/internal/shared"
)

const aiFailurePayloadExcerptMaxChars = 200

// AIProviderFailure carries structured provider diagnostics for retry logging.
type AIProviderFailure struct {
	HTTPStatus     int
	ErrorType      string
	ErrorMessage   string
	PayloadExcerpt string
}

func (e *AIProviderFailure) Error() string {
	parts := make([]string, 0, 3)
	if e.HTTPStatus > 0 {
		parts = append(parts, fmt.Sprintf("status=%d", e.HTTPStatus))
	}
	if strings.TrimSpace(e.ErrorType) != "" {
		parts = append(parts, "type="+strings.TrimSpace(e.ErrorType))
	}
	if strings.TrimSpace(e.ErrorMessage) != "" {
		parts = append(parts, "message="+strings.TrimSpace(e.ErrorMessage))
	}
	if len(parts) == 0 {
		return "AI provider failure"
	}
	return "AI provider failure: " + strings.Join(parts, " ")
}

func truncateWithPrefix(raw string, max int) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if max <= 0 || len(trimmed) <= max {
		return trimmed
	}
	return "[truncated] " + trimmed[:max]
}

func parseClaudeErrorEnvelope(body []byte) (errorType, message string, ok bool) {
	var env struct {
		Type  string `json:"type"`
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return "", "", false
	}

	errorType = strings.TrimSpace(env.Error.Type)
	message = strings.TrimSpace(env.Error.Message)
	if errorType == "" && message == "" {
		return "", "", false
	}
	return errorType, message, true
}

// InterviewAIClientConfig holds everything needed to build and call the AI API.
type InterviewAIClientConfig struct {
	BaseURL                 string              // "https://api.anthropic.com" or mock URL
	APIKey                  string              // Anthropic API key
	Model                   string              // e.g. "claude-sonnet-4-20250514"
	MaxTokens               int                 // e.g. 1024
	AllowSensitiveDebugLogs bool                // Allows sensitive prompt/response debug logs when true
	SystemPrompt            string              // loaded from AI_INTERVIEW_SYSTEM_PROMPT env
	PromptLastQuestion      string              // appended for per-area last follow-up, time pressure, or midpoint pacing
	PromptClosing           string              // appended when whole-interview time is almost up
	PromptOpeningTurn       string              // opening-turn instruction for current-turn payload
	LastQuestionSeconds     int                 // time threshold for last-question prompt (e.g. 30)
	ClosingSeconds          int                 // time threshold for closing prompt (e.g. 15)
	MidpointAreaIndex       int                 // area index that defines the pacing midpoint (e.g. 3 = nexus)
	EnablePromptCaching     bool                // enables prompt caching for Claude requests
	TimeoutSeconds          int                 // HTTP timeout for AI API calls (e.g. 30)
	AreaConfigs             []config.AreaConfig // loaded from AI_AREA_CONFIG env
}

// promptComposer holds prompt pacing config shared by AI providers.
type promptComposer struct {
	systemPrompt      string
	promptLastQ       string
	promptClosing     string
	lastQSeconds      int
	closingSeconds    int
	midpointAreaIndex int
}

func (pc *promptComposer) composeSystem() string {
	return pc.systemPrompt
}

// composePriority returns a turn-local priority snippet without mutating the system prompt.
// Priority cascade (highest wins):
//  1. Closing (time-only): timeRemainingS <= closingSeconds — whole interview wrapping up
//  2. LastQ (time): timeRemainingS <= lastQSeconds — time pressure
//  3. LastQ (per-area): followUpsRemaining <= 1 — last follow-up for this criterion
//  4. LastQ (midpoint pacing): at/before midpoint area AND used 2/3 of total budget
func (pc *promptComposer) composePriority(turnCtx *AITurnContext) string {
	// 1. Closing — whole-interview time pressure only.
	if pc.promptClosing != "" && turnCtx.TimeRemainingS <= pc.closingSeconds {
		slog.Debug("appending closing prompt",
			"time_remaining_s", turnCtx.TimeRemainingS,
			"closing_seconds_threshold", pc.closingSeconds,
		)
		return pc.promptClosing
	}

	// 2. LastQ — whole-interview time pressure.
	if pc.promptLastQ != "" && turnCtx.TimeRemainingS <= pc.lastQSeconds {
		slog.Debug("appending last-question prompt (time)",
			"time_remaining_s", turnCtx.TimeRemainingS,
			"last_q_seconds_threshold", pc.lastQSeconds,
		)
		return pc.promptLastQ
	}

	// 3. LastQ — per-area follow-up budget.
	if pc.promptLastQ != "" && turnCtx.FollowUpsRemaining <= 1 {
		slog.Debug("appending last-question prompt (per-area)",
			"follow_ups_remaining", turnCtx.FollowUpsRemaining,
		)
		return pc.promptLastQ
	}

	// 4. LastQ — midpoint pacing: still on an early area and 2/3 of time used.
	if pc.promptLastQ != "" && turnCtx.TotalBudgetS > 0 &&
		turnCtx.CurrentAreaIndex <= pc.midpointAreaIndex &&
		turnCtx.TimeRemainingS <= turnCtx.TotalBudgetS/3 {
		slog.Debug("appending last-question prompt (midpoint pacing)",
			"area_index", turnCtx.CurrentAreaIndex,
			"midpoint_area_index", pc.midpointAreaIndex,
			"time_remaining_s", turnCtx.TimeRemainingS,
			"total_budget_s", turnCtx.TotalBudgetS,
			"one_third_budget", turnCtx.TotalBudgetS/3,
		)
		return pc.promptLastQ
	}

	return ""
}

// HTTPInterviewAIClient implements InterviewAIClient by calling the Claude Messages API.
type HTTPInterviewAIClient struct {
	baseURL                 string
	apiKey                  string
	model                   string
	maxTokens               int
	timeoutSeconds          int
	allowSensitiveDebugLogs bool
	areaConfigs             []config.AreaConfig
	openingTurnPrompt       string
	enablePromptCaching     bool
	outputSchema            map[string]interface{}
	promptComposer          promptComposer
	client                  *http.Client
}

// NewHTTPInterviewAIClient creates an AI client with the given configuration.
func NewHTTPInterviewAIClient(cfg InterviewAIClientConfig) *HTTPInterviewAIClient {
	return &HTTPInterviewAIClient{
		baseURL:                 cfg.BaseURL,
		apiKey:                  cfg.APIKey,
		model:                   cfg.Model,
		maxTokens:               cfg.MaxTokens,
		timeoutSeconds:          cfg.TimeoutSeconds,
		allowSensitiveDebugLogs: cfg.AllowSensitiveDebugLogs,
		areaConfigs:             cfg.AreaConfigs,
		openingTurnPrompt:       cfg.PromptOpeningTurn,
		enablePromptCaching:     cfg.EnablePromptCaching,
		outputSchema:            buildOutputSchema(),
		promptComposer: promptComposer{
			systemPrompt:      cfg.SystemPrompt,
			promptLastQ:       cfg.PromptLastQuestion,
			promptClosing:     cfg.PromptClosing,
			lastQSeconds:      cfg.LastQuestionSeconds,
			closingSeconds:    cfg.ClosingSeconds,
			midpointAreaIndex: cfg.MidpointAreaIndex,
		},
		client: &http.Client{Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second},
	}
}

// GenerateTurn builds the full Claude API request, sends it, and parses the response.
func (c *HTTPInterviewAIClient) GenerateTurn(ctx context.Context, turnCtx *AITurnContext) (*AIResponse, error) {
	priorityPrompt := c.promptComposer.composePriority(turnCtx)
	systemBlocks := buildClaudeSystemBlocks(c.promptComposer.composeSystem())
	messages, turnUserMessage := buildClaudeMessages(turnCtx, priorityPrompt, c.openingTurnPrompt)
	applyClaudePromptCaching(systemBlocks, messages, turnCtx, c.enablePromptCaching)

	requestBody := map[string]interface{}{
		"model":      c.model,
		"max_tokens": c.maxTokens,
		"system":     systemBlocks,
		"messages":   messages,
	}

	// Only include output_config if we have a schema (skip for mock server compatibility).
	if c.outputSchema != nil {
		requestBody["output_config"] = c.outputSchema
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := c.baseURL + "/v1/messages"
	slog.Debug("calling AI API",
		"url", url,
		"area", turnCtx.CurrentAreaSlug,
		"model", c.model,
		"sensitive_debug_logs_enabled", c.allowSensitiveDebugLogs,
	)
	if c.allowSensitiveDebugLogs {
		shared.DebugTextBlock("Claude request system prompt", c.promptComposer.composeSystem())
		shared.DebugTextBlock("Claude request turn user message", turnUserMessage)
		if requestMessages, ok := requestBody["messages"].([]map[string]interface{}); ok {
			shared.DebugChatMessages("Claude request messages", requestMessages)
		}
		shared.DebugJSON("Claude request body", requestBody)
	}

	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(c.timeoutSeconds)*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("x-api-key", c.apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	}

	start := time.Now()
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, &AIProviderFailure{
			ErrorType:    "transport_error",
			ErrorMessage: err.Error(),
		}
	}
	defer resp.Body.Close()

	slog.Debug("AI API responded", "status", resp.StatusCode, "duration", time.Since(start))

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &AIProviderFailure{
			HTTPStatus:   resp.StatusCode,
			ErrorType:    "response_read_error",
			ErrorMessage: err.Error(),
		}
	}
	if c.allowSensitiveDebugLogs {
		shared.DebugJSONText("Claude API raw response body", string(respBody))
	}

	if resp.StatusCode != http.StatusOK {
		payloadExcerpt := truncateWithPrefix(string(respBody), aiFailurePayloadExcerptMaxChars)
		errorType, errorMessage, ok := parseClaudeErrorEnvelope(respBody)
		if !ok {
			errorType = "http_error"
			errorMessage = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
		return nil, &AIProviderFailure{
			HTTPStatus:     resp.StatusCode,
			ErrorType:      errorType,
			ErrorMessage:   errorMessage,
			PayloadExcerpt: payloadExcerpt,
		}
	}

	var apiResp ClaudeAPIResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, &AIProviderFailure{
			HTTPStatus:     resp.StatusCode,
			ErrorType:      "response_decode_error",
			ErrorMessage:   err.Error(),
			PayloadExcerpt: truncateWithPrefix(string(respBody), aiFailurePayloadExcerptMaxChars),
		}
	}

	if c.allowSensitiveDebugLogs {
		shared.DebugJSON("Claude API raw response", apiResp)
	}
	slog.Info("Claude API usage",
		"phase", "interview",
		"area", turnCtx.CurrentAreaSlug,
		"is_opening_turn", turnCtx.IsOpeningTurn,
		"model", c.model,
		"input_tokens", apiResp.Usage.InputTokens,
		"output_tokens", apiResp.Usage.OutputTokens,
		"cache_creation_input_tokens", apiResp.Usage.CacheCreationInputTokens,
		"cache_read_input_tokens", apiResp.Usage.CacheReadInputTokens,
	)
	result, err := parseAIResponse(&apiResp)
	if err == nil {
		return result, nil
	}

	var aiFailure *AIProviderFailure
	if errors.As(err, &aiFailure) {
		if aiFailure.HTTPStatus <= 0 {
			aiFailure.HTTPStatus = resp.StatusCode
		}
		if strings.TrimSpace(aiFailure.PayloadExcerpt) == "" {
			aiFailure.PayloadExcerpt = truncateWithPrefix(string(respBody), aiFailurePayloadExcerptMaxChars)
		}
		return nil, aiFailure
	}
	return nil, err
}

// parseAIResponse extracts the AIResponse from a Claude API envelope.
func parseAIResponse(apiResp *ClaudeAPIResponse) (*AIResponse, error) {
	if apiResp.StopReason == "max_tokens" {
		return nil, &AIProviderFailure{
			HTTPStatus:   http.StatusOK,
			ErrorType:    "stop_reason_max_tokens",
			ErrorMessage: "response truncated due to max_tokens",
		}
	}
	if apiResp.StopReason == "refusal" {
		return nil, &AIProviderFailure{
			HTTPStatus:   http.StatusOK,
			ErrorType:    "stop_reason_refusal",
			ErrorMessage: "model refused to answer",
		}
	}

	if len(apiResp.Content) == 0 {
		return nil, &AIProviderFailure{
			HTTPStatus:   http.StatusOK,
			ErrorType:    "empty_content",
			ErrorMessage: "empty content in API response",
		}
	}

	jsonStr := apiResp.Content[0].Text

	var result AIResponse
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, &AIProviderFailure{
			HTTPStatus:     http.StatusOK,
			ErrorType:      "parse_error",
			ErrorMessage:   err.Error(),
			PayloadExcerpt: truncateWithPrefix(jsonStr, aiFailurePayloadExcerptMaxChars),
		}
	}
	if err := validateAIResponse(&result); err != nil {
		return nil, &AIProviderFailure{
			HTTPStatus:     http.StatusOK,
			ErrorType:      "invalid_response",
			ErrorMessage:   err.Error(),
			PayloadExcerpt: truncateWithPrefix(jsonStr, aiFailurePayloadExcerptMaxChars),
		}
	}

	slog.Debug("AI response parsed",
		"has_evaluation", result.Evaluation != nil,
		"next_question", result.NextQuestion,
	)
	if result.Evaluation != nil {
		slog.Debug("AI evaluation",
			"status", result.Evaluation.CurrentCriterion.Status,
			"recommendation", result.Evaluation.CurrentCriterion.Recommendation,
			"evidence", result.Evaluation.CurrentCriterion.EvidenceSummary,
			"other_criteria_count", len(result.Evaluation.OtherCriteriaAddressed),
		)
	}

	return &result, nil
}

func validateAIResponse(result *AIResponse) error {
	if strings.TrimSpace(result.NextQuestion) == "" {
		return fmt.Errorf("invalid AI response: next_question is empty")
	}
	if result.Evaluation == nil {
		return nil
	}

	switch result.Evaluation.CurrentCriterion.Status {
	case CriterionStatusSufficient, CriterionStatusPartial, CriterionStatusInsufficient:
	default:
		return fmt.Errorf("invalid AI response: unsupported current_criterion.status %q", result.Evaluation.CurrentCriterion.Status)
	}

	switch result.Evaluation.CurrentCriterion.Recommendation {
	case CriterionRecFollowUp, CriterionRecMoveOn:
	default:
		return fmt.Errorf("invalid AI response: unsupported current_criterion.recommendation %q", result.Evaluation.CurrentCriterion.Recommendation)
	}

	return nil
}

// buildOutputSchema creates the static JSON schema for Claude's output_config.
func buildOutputSchema() map[string]interface{} {
	return map[string]interface{}{
		"format": map[string]interface{}{
			"type": "json_schema",
			"schema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"evaluation": map[string]interface{}{
						"type": []string{"object", "null"},
						"properties": map[string]interface{}{
							"current_criterion": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"id":               map[string]interface{}{"type": "integer"},
									"status":           map[string]interface{}{"type": "string", "enum": []string{"sufficient", "partially_sufficient", "insufficient"}},
									"evidence_summary": map[string]interface{}{"type": "string"},
									"recommendation":   map[string]interface{}{"type": "string", "enum": []string{"follow_up", "move_on"}},
								},
								"required":             []string{"id", "status", "evidence_summary", "recommendation"},
								"additionalProperties": false,
							},
							"other_criteria_addressed": map[string]interface{}{
								"type": "array",
								"items": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"id":               map[string]interface{}{"type": "integer"},
										"name":             map[string]interface{}{"type": "string"},
										"evidence_summary": map[string]interface{}{"type": "string"},
										"confidence":       map[string]interface{}{"type": "string", "enum": []string{"partial", "strong"}},
									},
									"required":             []string{"id", "name", "evidence_summary", "confidence"},
									"additionalProperties": false,
								},
							},
						},
						"required":             []string{"current_criterion", "other_criteria_addressed"},
						"additionalProperties": false,
					},
					"next_question": map[string]interface{}{"type": "string"},
				},
				"required":             []string{"evaluation", "next_question"},
				"additionalProperties": false,
			},
		},
	}
}
