package report

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/afirmativo/backend/internal/shared"
	"github.com/afirmativo/backend/internal/vertexai"
)

// VertexReportAIClientConfig holds config for the Vertex report AI client.
type VertexReportAIClientConfig struct {
	Model                   string
	MaxTokens               int
	ReportPrompt            string
	AllowSensitiveDebugLogs bool
	VertexClient            *vertexai.Client
}

// VertexReportAIClient implements ReportAIClient using Vertex AI.
type VertexReportAIClient struct {
	model                   string
	maxTokens               int
	reportPrompt            string
	allowSensitiveDebugLogs bool
	vertexClient            *vertexai.Client
}

// NewVertexReportAIClient creates a new report AI client backed by Vertex.
func NewVertexReportAIClient(cfg VertexReportAIClientConfig) *VertexReportAIClient {
	return &VertexReportAIClient{
		model:                   cfg.Model,
		maxTokens:               cfg.MaxTokens,
		reportPrompt:            cfg.ReportPrompt,
		allowSensitiveDebugLogs: cfg.AllowSensitiveDebugLogs,
		vertexClient:            cfg.VertexClient,
	}
}

// GenerateReport sends area summaries to Vertex and parses structured JSON output.
func (c *VertexReportAIClient) GenerateReport(ctx context.Context, areaSummaries []AreaSummary, openFloorTranscript string) (*ReportAIResponse, error) {
	userContent := buildReportUserMessage(areaSummaries, openFloorTranscript)
	systemInstruction := vertexai.NewTextContent("", c.reportPrompt)

	cacheOutcome, cacheErr := c.vertexClient.EnsureCachedContent(ctx, vertexai.CacheSpec{
		Key:               buildReportVertexCacheKey(c.model, c.reportPrompt),
		Model:             c.model,
		DisplayName:       "report-system-prompt",
		SystemInstruction: &systemInstruction,
	})
	if cacheErr != nil {
		slog.Warn("Vertex report explicit cache unavailable",
			"model", c.model,
			"error", cacheErr,
		)
		cacheOutcome = &vertexai.CacheOutcome{Mode: "implicit_only_cache_error"}
	}
	if cacheOutcome == nil {
		cacheOutcome = &vertexai.CacheOutcome{Mode: "implicit_only"}
	}

	requestBody := vertexai.GenerateContentRequest{
		Contents: []vertexai.Content{
			vertexai.NewTextContent("user", userContent),
		},
		GenerationConfig: vertexai.GenerationConfig{
			MaxOutputTokens:  c.maxTokens,
			ResponseMIMEType: "application/json",
			ResponseSchema:   buildVertexReportResponseSchema(),
		},
	}
	if cacheOutcome.CachedContentName != "" {
		requestBody.CachedContent = cacheOutcome.CachedContentName
	} else {
		requestBody.SystemInstruction = &systemInstruction
	}

	slog.Debug("calling AI API for report",
		"provider", "vertex",
		"model", c.model,
		"cache_mode", cacheOutcome.Mode,
		"sensitive_debug_logs_enabled", c.allowSensitiveDebugLogs,
	)
	if c.allowSensitiveDebugLogs {
		shared.DebugJSON("report Vertex request body", requestBody)
	}

	resp, err := c.vertexClient.GenerateContent(ctx, c.model, requestBody)
	if err != nil {
		return nil, toReportVertexError(err)
	}
	if c.allowSensitiveDebugLogs {
		shared.DebugJSON("report Vertex raw response", resp)
	}

	jsonStr, err := vertexReportResponseText(resp)
	if err != nil {
		return nil, err
	}
	result, err := parseReportAIResponseJSON(jsonStr)
	if err != nil {
		return nil, err
	}

	slog.Debug("report AI response parsed",
		"areas_of_clarity_count", len(result.AreasOfClarity),
		"areas_to_develop_further_count", len(result.AreasToDevelopFurther),
		"recommendation_len", len(result.Recommendation),
		"areas_of_clarity_es_count", len(result.AreasOfClarityEs),
		"areas_to_develop_further_es_count", len(result.AreasToDevelopFurtherEs),
		"recommendation_es_len", len(result.RecommendationEs),
	)
	slog.Info("AI API usage",
		"phase", "report",
		"provider", "vertex",
		"model", c.model,
		"model_version", resp.ModelVersion,
		"prompt_tokens", resp.UsageMetadata.PromptTokenCount,
		"candidate_tokens", resp.UsageMetadata.CandidatesTokenCount,
		"total_tokens", resp.UsageMetadata.TotalTokenCount,
		"cached_content_tokens", resp.UsageMetadata.CachedContentTokenCount,
		"cache_mode", cacheOutcome.Mode,
	)

	return result, nil
}

func buildReportVertexCacheKey(model, reportPrompt string) string {
	hash := sha256.Sum256([]byte(model + "\n" + reportPrompt))
	return "report:" + hex.EncodeToString(hash[:])
}

func buildVertexReportResponseSchema() map[string]any {
	outputSchema := buildReportOutputSchema()
	format, _ := outputSchema["format"].(map[string]interface{})
	schema, _ := format["schema"].(map[string]interface{})
	return vertexai.NormalizeResponseSchema(schema)
}

func vertexReportResponseText(resp *vertexai.GenerateContentResponse) (string, error) {
	if len(resp.Candidates) == 0 {
		if reason := strings.TrimSpace(resp.PromptFeedback.BlockReason); reason != "" {
			return "", fmt.Errorf("report prompt blocked: %s", reason)
		}
		return "", fmt.Errorf("empty candidates in Vertex response")
	}

	first := resp.Candidates[0]
	switch strings.TrimSpace(first.FinishReason) {
	case "", "STOP":
	case "MAX_TOKENS":
		return "", fmt.Errorf("response truncated: finish_reason=MAX_TOKENS")
	default:
		return "", fmt.Errorf("Vertex report generation stopped with %s", first.FinishReason)
	}

	if len(first.Content.Parts) == 0 || strings.TrimSpace(first.Content.Parts[0].Text) == "" {
		return "", fmt.Errorf("empty text parts in Vertex response")
	}
	return first.Content.Parts[0].Text, nil
}

func toReportVertexError(err error) error {
	var vertexErr *vertexai.APIError
	if !errors.As(err, &vertexErr) {
		return fmt.Errorf("Vertex API call failed: %w", err)
	}
	if strings.TrimSpace(vertexErr.Message) != "" {
		return fmt.Errorf("Vertex API returned status %d: %s", vertexErr.StatusCode, vertexErr.Message)
	}
	return fmt.Errorf("Vertex API returned status %d", vertexErr.StatusCode)
}
