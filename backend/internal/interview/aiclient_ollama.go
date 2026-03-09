package interview

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/afirmativo/backend/internal/config"
	"github.com/afirmativo/backend/internal/shared"
)

// OllamaInterviewAIClientConfig holds configuration for the Ollama interview AI client.
type OllamaInterviewAIClientConfig struct {
	BaseURL                 string
	Model                   string
	MaxTokens               int
	Temperature             float64
	AllowSensitiveDebugLogs bool
	SystemPrompt            string
	OutputFormatPrompt      string
	PromptLastQuestion      string
	PromptClosing           string
	PromptOpeningTurn       string
	LastQuestionSeconds     int
	ClosingSeconds          int
	MidpointAreaIndex       int
	TimeoutSeconds          int
	AreaConfigs             []config.AreaConfig
}

// OllamaInterviewAIClient implements InterviewAIClient using Ollama's OpenAI-compatible endpoint.
type OllamaInterviewAIClient struct {
	baseURL                 string
	model                   string
	maxTokens               int
	temperature             float64
	systemPrompt            string
	outputFormatPrompt      string
	openingTurnPrompt       string
	areaConfigs             []config.AreaConfig
	allowSensitiveDebugLogs bool
	client                  *http.Client
	promptComposer          promptComposer
}

// NewOllamaInterviewAIClient creates a new Ollama-backed interview AI client.
func NewOllamaInterviewAIClient(cfg OllamaInterviewAIClientConfig) *OllamaInterviewAIClient {
	return &OllamaInterviewAIClient{
		baseURL:                 cfg.BaseURL,
		model:                   cfg.Model,
		maxTokens:               cfg.MaxTokens,
		temperature:             cfg.Temperature,
		allowSensitiveDebugLogs: cfg.AllowSensitiveDebugLogs,
		systemPrompt:            cfg.SystemPrompt,
		outputFormatPrompt:      cfg.OutputFormatPrompt,
		openingTurnPrompt:       cfg.PromptOpeningTurn,
		areaConfigs:             cfg.AreaConfigs,
		client:                  &http.Client{Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second},
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

// GenerateTurn sends the interview turn context to Ollama and parses structured JSON output.
func (c *OllamaInterviewAIClient) GenerateTurn(ctx context.Context, turnCtx *AITurnContext) (*AIResponse, error) {
	priorityPrompt := c.promptComposer.composePriority(turnCtx)
	messages, turnUserMessage := buildOllamaMessages(turnCtx, priorityPrompt, c.openingTurnPrompt)
	systemPrompt := c.promptComposer.composeSystem()
	if c.outputFormatPrompt != "" {
		systemPrompt += "\n\n" + c.outputFormatPrompt
	}

	requestBody := map[string]interface{}{
		"model":      c.model,
		"max_tokens": c.maxTokens,
		"messages": append([]map[string]interface{}{
			{"role": "system", "content": systemPrompt},
		}, messages...),
		"temperature":     c.temperature,
		"response_format": map[string]interface{}{"type": "json_object"},
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := strings.TrimRight(c.baseURL, "/") + "/v1/chat/completions"
	slog.Debug("calling Ollama API",
		"url", url,
		"area", turnCtx.CurrentAreaSlug,
		"model", c.model,
		"sensitive_debug_logs_enabled", c.allowSensitiveDebugLogs,
	)
	if c.allowSensitiveDebugLogs {
		shared.DebugTextBlock("Ollama request system prompt", systemPrompt)
		shared.DebugTextBlock("Ollama request turn user message", turnUserMessage)
		if requestMessages, ok := requestBody["messages"].([]map[string]interface{}); ok {
			shared.DebugChatMessages("Ollama request messages", requestMessages)
		}
		shared.DebugJSON("Ollama request body", requestBody)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Ollama API call failed: %w", err)
	}
	defer resp.Body.Close()

	slog.Debug("Ollama API responded", "status", resp.StatusCode, "duration", time.Since(start))

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Ollama API returned status %d", resp.StatusCode)
	}

	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decode API response: %w", err)
	}

	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("empty choices in API response")
	}
	if apiResp.Choices[0].FinishReason == "length" {
		return nil, fmt.Errorf("response truncated: finish_reason=length")
	}

	jsonStr := apiResp.Choices[0].Message.Content
	if strings.TrimSpace(jsonStr) == "" {
		return nil, fmt.Errorf("empty message content in API response")
	}

	var result AIResponse
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("parse AI response JSON: %w", err)
	}
	if turnCtx.IsOpeningTurn {
		// Opening turns only need next_question; some models emit partial
		// evaluation objects even when instructed to return null.
		result.Evaluation = nil
	}
	if err := validateAIResponse(&result); err != nil {
		return nil, err
	}

	slog.Debug("Ollama response parsed",
		"has_evaluation", result.Evaluation != nil,
		"next_question", result.NextQuestion,
	)
	if result.Evaluation != nil {
		slog.Debug("Ollama evaluation",
			"status", result.Evaluation.CurrentCriterion.Status,
			"recommendation", result.Evaluation.CurrentCriterion.Recommendation,
			"evidence", result.Evaluation.CurrentCriterion.EvidenceSummary,
			"other_criteria_count", len(result.Evaluation.OtherCriteriaAddressed),
		)
	}

	return &result, nil
}
