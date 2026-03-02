// HTTP client that calls the Claude API (or mock server) to evaluate answers
// and generate the next interview question.
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
)

// AIClientConfig holds everything needed to build and call the AI API.
type AIClientConfig struct {
	BaseURL           string              // "https://api.anthropic.com" or mock URL
	APIKey            string              // Anthropic API key
	Model             string              // e.g. "claude-sonnet-4-20250514"
	MaxTokens         int                 // e.g. 1024
	SystemPrompt      string              // loaded from AI_SYSTEM_PROMPT env
	PromptLastQ       string              // appended for per-area last follow-up, time pressure, or midpoint pacing
	PromptClosing     string              // appended when whole-interview time is almost up
	LastQSeconds      int                 // time threshold for last-question prompt (e.g. 30)
	ClosingSeconds    int                 // time threshold for closing prompt (e.g. 15)
	MidpointAreaIndex int                 // area index that defines the pacing midpoint (e.g. 3 = nexus)
	TimeoutSeconds    int                 // HTTP timeout for AI API calls (e.g. 30)
	AreaConfigs       []config.AreaConfig // loaded from AI_AREA_CONFIG env
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

// compose builds the final system prompt by appending urgency snippets.
// Priority cascade (highest wins):
//  1. Closing (time-only): timeRemainingS <= closingSeconds — whole interview wrapping up
//  2. LastQ (time): timeRemainingS <= lastQSeconds — time pressure
//  3. LastQ (per-area): followUpsRemaining <= 1 — last follow-up for this criterion
//  4. LastQ (midpoint pacing): at/before midpoint area AND used 2/3 of total budget
func (pc *promptComposer) compose(turnCtx *AITurnContext) string {
	prompt := pc.systemPrompt

	// 1. Closing — whole-interview time pressure only.
	if pc.promptClosing != "" && turnCtx.TimeRemainingS <= pc.closingSeconds {
		slog.Debug("appending closing prompt",
			"time_remaining_s", turnCtx.TimeRemainingS,
			"closing_seconds_threshold", pc.closingSeconds,
		)
		prompt += "\n\n" + pc.promptClosing
		return prompt
	}

	// 2. LastQ — whole-interview time pressure.
	if pc.promptLastQ != "" && turnCtx.TimeRemainingS <= pc.lastQSeconds {
		slog.Debug("appending last-question prompt (time)",
			"time_remaining_s", turnCtx.TimeRemainingS,
			"last_q_seconds_threshold", pc.lastQSeconds,
		)
		prompt += "\n\n" + pc.promptLastQ
		return prompt
	}

	// 3. LastQ — per-area follow-up budget.
	if pc.promptLastQ != "" && turnCtx.FollowUpsRemaining <= 1 {
		slog.Debug("appending last-question prompt (per-area)",
			"follow_ups_remaining", turnCtx.FollowUpsRemaining,
		)
		prompt += "\n\n" + pc.promptLastQ
		return prompt
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
		prompt += "\n\n" + pc.promptLastQ
		return prompt
	}

	return prompt
}

// HTTPAIClient implements AIClient by calling the Claude Messages API.
type HTTPAIClient struct {
	baseURL        string
	apiKey         string
	model          string
	maxTokens      int
	timeoutSeconds int
	areaConfigs    []config.AreaConfig
	outputSchema   map[string]interface{}
	promptComposer promptComposer
	client         *http.Client
}

// NewHTTPAIClient creates an AI client with the given configuration.
func NewHTTPAIClient(cfg AIClientConfig) *HTTPAIClient {
	return &HTTPAIClient{
		baseURL:        cfg.BaseURL,
		apiKey:         cfg.APIKey,
		model:          cfg.Model,
		maxTokens:      cfg.MaxTokens,
		timeoutSeconds: cfg.TimeoutSeconds,
		areaConfigs:    cfg.AreaConfigs,
		outputSchema:   buildOutputSchema(),
		promptComposer: promptComposer{
			systemPrompt:      cfg.SystemPrompt,
			promptLastQ:       cfg.PromptLastQ,
			promptClosing:     cfg.PromptClosing,
			lastQSeconds:      cfg.LastQSeconds,
			closingSeconds:    cfg.ClosingSeconds,
			midpointAreaIndex: cfg.MidpointAreaIndex,
		},
		client: &http.Client{Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second},
	}
}

// CallAI builds the full Claude API request, sends it, and parses the response.
func (c *HTTPAIClient) CallAI(ctx context.Context, turnCtx *AITurnContext) (*AIResponse, error) {
	userContent := buildUserMessage(turnCtx)
	systemPrompt := c.promptComposer.compose(turnCtx)

	requestBody := map[string]interface{}{
		"model":      c.model,
		"max_tokens": c.maxTokens,
		"system":     systemPrompt,
		"messages": []map[string]interface{}{
			{"role": "user", "content": userContent},
		},
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
	slog.Debug("calling AI API", "url", url, "area", turnCtx.CurrentAreaSlug, "model", c.model)
	slog.Debug("AI request user message", "content", userContent)
	slog.Debug("AI request body", "body", string(bodyBytes))

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
		return nil, fmt.Errorf("AI API call failed: %w", err)
	}
	defer resp.Body.Close()

	slog.Debug("AI API responded", "status", resp.StatusCode, "duration", time.Since(start))

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("AI API returned status %d", resp.StatusCode)
	}

	var apiResp ClaudeAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decode API response: %w", err)
	}

	// Log raw API response envelope in debug mode.
	if rawResp, mErr := json.Marshal(apiResp); mErr == nil {
		slog.Debug("AI API raw response", "response", string(rawResp))
	}

	return parseAIResponse(&apiResp)
}

// parseAIResponse extracts the AIResponse from a Claude API envelope.
func parseAIResponse(apiResp *ClaudeAPIResponse) (*AIResponse, error) {
	if apiResp.StopReason == "max_tokens" {
		return nil, fmt.Errorf("response truncated: stop_reason=max_tokens")
	}

	if len(apiResp.Content) == 0 {
		return nil, fmt.Errorf("empty content in API response")
	}

	jsonStr := apiResp.Content[0].Text

	var result AIResponse
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("parse AI response JSON: %w", err)
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
	case "sufficient", "partially_sufficient", "insufficient":
	default:
		return fmt.Errorf("invalid AI response: unsupported current_criterion.status %q", result.Evaluation.CurrentCriterion.Status)
	}

	switch result.Evaluation.CurrentCriterion.Recommendation {
	case "follow_up", "move_on":
	default:
		return fmt.Errorf("invalid AI response: unsupported current_criterion.recommendation %q", result.Evaluation.CurrentCriterion.Recommendation)
	}

	return nil
}

// buildUserMessage constructs the user prompt with all interview context.
func buildUserMessage(tc *AITurnContext) string {
	criteriaJSON, _ := json.MarshalIndent(tc.CriteriaCoverage, "", "  ")
	transcriptJSON, _ := json.MarshalIndent(tc.Transcript, "", "  ")

	return fmt.Sprintf(`CURRENT CRITERION:
{
  "id": %d,
  "name": "%s",
  "description": "%s",
  "sufficiency_requirements": "%s"
}

CRITERION STATUS: %s
IS PRE-ADDRESSED: %t
FOLLOW-UPS REMAINING FOR THIS CRITERION: %d

INTERVIEW PROGRESS:
- Time remaining: %d seconds
- Questions remaining: %d
- Criteria remaining (including current): %d

CRITERIA COVERAGE:
%s

TRANSCRIPT:
%s

Please evaluate the candidate's most recent answer against the current criterion and generate the next question.`,
		tc.CurrentAreaID,
		tc.CurrentAreaLabel,
		tc.Description,
		tc.SufficiencyReqs,
		tc.AreaStatus,
		tc.IsPreAddressed,
		tc.FollowUpsRemaining,
		tc.TimeRemainingS,
		tc.QuestionsRemaining,
		tc.CriteriaRemaining,
		string(criteriaJSON),
		string(transcriptJSON),
	)
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
