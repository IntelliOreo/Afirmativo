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

// OllamaAIClientConfig holds configuration for the Ollama interview AI client.
type OllamaAIClientConfig struct {
	BaseURL            string
	Model              string
	SystemPrompt       string
	OutputFormatPrompt string
	PromptLastQ        string
	PromptClosing      string
	LastQSeconds       int
	ClosingSeconds     int
	MidpointAreaIndex  int
	TimeoutSeconds     int
	AreaConfigs        []config.AreaConfig
}

// OllamaAIClient implements AIClient using Ollama's OpenAI-compatible endpoint.
type OllamaAIClient struct {
	baseURL            string
	model              string
	systemPrompt       string
	outputFormatPrompt string
	promptLastQ        string
	promptClosing      string
	lastQSeconds       int
	closingSeconds     int
	midpointAreaIndex  int
	areaConfigs        []config.AreaConfig
	client             *http.Client
	promptComposer     promptComposer
}

// NewOllamaAIClient creates a new Ollama-backed interview AI client.
func NewOllamaAIClient(cfg OllamaAIClientConfig) *OllamaAIClient {
	return &OllamaAIClient{
		baseURL:            cfg.BaseURL,
		model:              cfg.Model,
		systemPrompt:       cfg.SystemPrompt,
		outputFormatPrompt: cfg.OutputFormatPrompt,
		promptLastQ:        cfg.PromptLastQ,
		promptClosing:      cfg.PromptClosing,
		lastQSeconds:       cfg.LastQSeconds,
		closingSeconds:     cfg.ClosingSeconds,
		midpointAreaIndex:  cfg.MidpointAreaIndex,
		areaConfigs:        cfg.AreaConfigs,
		client:             &http.Client{Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second},
		promptComposer: promptComposer{
			systemPrompt:      cfg.SystemPrompt,
			promptLastQ:       cfg.PromptLastQ,
			promptClosing:     cfg.PromptClosing,
			lastQSeconds:      cfg.LastQSeconds,
			closingSeconds:    cfg.ClosingSeconds,
			midpointAreaIndex: cfg.MidpointAreaIndex,
		},
	}
}

// CallAI sends the interview turn context to Ollama and parses structured JSON output.
func (c *OllamaAIClient) CallAI(ctx context.Context, turnCtx *AITurnContext) (*AIResponse, error) {
	userContent := buildUserMessage(turnCtx)
	systemPrompt := c.promptComposer.compose(turnCtx)
	if c.outputFormatPrompt != "" {
		systemPrompt += "\n\n" + c.outputFormatPrompt
	}

	requestBody := map[string]interface{}{
		"model": c.model,
		"messages": []map[string]interface{}{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userContent},
		},
		"temperature":     0.3,
		"response_format": map[string]interface{}{"type": "json_object"},
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := strings.TrimRight(c.baseURL, "/") + "/v1/chat/completions"
	slog.Debug("calling Ollama API", "url", url, "area", turnCtx.CurrentAreaSlug, "model", c.model)
	shared.DebugTextBlock("Ollama request user message", userContent)
	shared.DebugJSON("Ollama request body", requestBody)

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
