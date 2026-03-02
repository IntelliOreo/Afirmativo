// Package config defines the application configuration struct,
// loads values from environment variables, and validates them at startup.
// Fails fast on missing or invalid configuration.
package config

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strconv"
)

// AreaConfig holds the rubric and fallback for a single interview criterion.
// Loaded from the AI_AREA_CONFIG env var (JSON array).
type AreaConfig struct {
	ID                      int    `json:"id"`
	Slug                    string `json:"slug"`
	Label                   string `json:"label"`
	Description             string `json:"description"`
	SufficiencyRequirements string `json:"sufficiency_requirements"`
	FallbackQuestion        string `json:"fallback_question"`
}

// Config holds all application configuration loaded from environment variables.
// In local dev, values come from .env via godotenv.
// In containers, values come from the runtime environment (e.g., Secret Manager).
type Config struct {
	Port               string
	FrontendURL        string
	DatabaseURL        string
	SessionExpiryHours int
	MockAPIURL         string // If non-empty, use this mock server instead of real AI APIs
	LogLevel           string // "debug", "info", "warn", "error" — defaults to "debug"

	// AI configuration — all AI instructions live here, not in Go code.
	AIProvider                              string       // "claude" (default) or "ollama"
	OllamaBaseURL                           string       // Base URL for Ollama OpenAI-compatible endpoint
	AISystemPrompt                          string       // Base system prompt sent to Claude/Ollama on every turn
	UnstructuredInterviewOutputFormatPrompt string       // Prompt instructions for unstructured providers to return interview JSON
	AIPromptLastQ                           string       // Appended when 1 follow-up remains OR time <= AILastQSeconds
	AIPromptClosing                         string       // Appended when 0 follow-ups remain OR time <= AIClosingSeconds
	AILastQSeconds                          int          // Time threshold (seconds) to trigger last-question prompt
	AIClosingSeconds                        int          // Time threshold (seconds) to trigger closing prompt
	AIMidpointAreaIdx                       int          // Area index defining the pacing midpoint (e.g. 3 = nexus)
	AIModel                                 string       // e.g. "claude-sonnet-4-20250514"
	AIMaxTokens                             int          // e.g. 1024
	AIAPIKey                                string       // Anthropic API key (not required for Ollama or when MOCK_API_URL is set)
	AITimeoutSeconds                        int          // HTTP timeout for AI API calls (default 30)
	AIReportPrompt                          string       // System prompt for report generation AI call
	UnstructuredReportOutputFormatPrompt    string       // Prompt instructions for unstructured providers to return report JSON
	AIReportMaxTokens                       int          // Max tokens for report AI call (default 2048)
	AreaConfigs                             []AreaConfig // Per-area rubrics loaded from AI_AREA_CONFIG JSON
}

// Load reads required environment variables and returns a validated Config.
// Returns an error if any required variable is missing.
func Load() (Config, error) {
	expiryStr := envOr("SESSION_EXPIRY_HOURS", "24")
	expiry, err := strconv.Atoi(expiryStr)
	if err != nil {
		return Config{}, fmt.Errorf("invalid SESSION_EXPIRY_HOURS: %w", err)
	}

	maxTokensStr := envOr("AI_MAX_TOKENS", "1024")
	maxTokens, err2 := strconv.Atoi(maxTokensStr)
	if err2 != nil {
		return Config{}, fmt.Errorf("invalid AI_MAX_TOKENS: %w", err2)
	}

	lastQStr := envOr("AI_LAST_Q_SECONDS", "30")
	lastQSec, err3 := strconv.Atoi(lastQStr)
	if err3 != nil {
		return Config{}, fmt.Errorf("invalid AI_LAST_Q_SECONDS: %w", err3)
	}

	closingStr := envOr("AI_CLOSING_SECONDS", "15")
	closingSec, err4 := strconv.Atoi(closingStr)
	if err4 != nil {
		return Config{}, fmt.Errorf("invalid AI_CLOSING_SECONDS: %w", err4)
	}

	midpointStr := envOr("AI_MIDPOINT_AREA_INDEX", "3")
	midpointIdx, err5 := strconv.Atoi(midpointStr)
	if err5 != nil {
		return Config{}, fmt.Errorf("invalid AI_MIDPOINT_AREA_INDEX: %w", err5)
	}

	aiTimeoutStr := envOr("AI_TIMEOUT_SECONDS", "30")
	aiTimeout, err6 := strconv.Atoi(aiTimeoutStr)
	if err6 != nil {
		return Config{}, fmt.Errorf("invalid AI_TIMEOUT_SECONDS: %w", err6)
	}

	reportMaxTokensStr := envOr("AI_REPORT_MAX_TOKENS", "2048")
	reportMaxTokens, err7 := strconv.Atoi(reportMaxTokensStr)
	if err7 != nil {
		return Config{}, fmt.Errorf("invalid AI_REPORT_MAX_TOKENS: %w", err7)
	}

	cfg := Config{
		Port:                                    envOr("PORT", "8080"),
		FrontendURL:                             envOr("FRONTEND_URL", "http://localhost:3000"),
		DatabaseURL:                             os.Getenv("DATABASE_URL"),
		SessionExpiryHours:                      expiry,
		MockAPIURL:                              os.Getenv("MOCK_API_URL"),
		LogLevel:                                envOr("LOG_LEVEL", "debug"),
		AIProvider:                              envOr("AI_PROVIDER", "claude"),
		OllamaBaseURL:                           envOr("OLLAMA_BASE_URL", "http://localhost:11434"),
		AISystemPrompt:                          os.Getenv("AI_SYSTEM_PROMPT"),
		UnstructuredInterviewOutputFormatPrompt: os.Getenv("UNSTRUCTURED_INTERVIEW_OUTPUT_FORMAT_PROMPT"),
		AIPromptLastQ:                           os.Getenv("AI_PROMPT_LAST_Q"),
		AIPromptClosing:                         os.Getenv("AI_PROMPT_CLOSING"),
		AILastQSeconds:                          lastQSec,
		AIClosingSeconds:                        closingSec,
		AIMidpointAreaIdx:                       midpointIdx,
		AIModel:                                 envOr("AI_MODEL", "claude-sonnet-4-20250514"),
		AIMaxTokens:                             maxTokens,
		AIAPIKey:                                os.Getenv("AI_API_KEY"),
		AITimeoutSeconds:                        aiTimeout,
		AIReportPrompt:                          os.Getenv("AI_REPORT_PROMPT"),
		UnstructuredReportOutputFormatPrompt:    os.Getenv("UNSTRUCTURED_REPORT_OUTPUT_FORMAT_PROMPT"),
		AIReportMaxTokens:                       reportMaxTokens,
	}

	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}

	if cfg.AIProvider != "claude" && cfg.AIProvider != "ollama" {
		return Config{}, fmt.Errorf("invalid AI_PROVIDER %q (expected \"claude\" or \"ollama\")", cfg.AIProvider)
	}

	// AI_API_KEY is required for Claude unless using mock server.
	if cfg.AIProvider != "ollama" && cfg.AIAPIKey == "" && cfg.MockAPIURL == "" {
		return Config{}, fmt.Errorf("AI_API_KEY is required (or set MOCK_API_URL for dev)")
	}
	if cfg.AIProvider == "ollama" {
		if cfg.MockAPIURL != "" {
			slog.Warn("MOCK_API_URL is ignored when AI_PROVIDER=ollama")
		}
		if cfg.UnstructuredInterviewOutputFormatPrompt == "" {
			slog.Warn("UNSTRUCTURED_INTERVIEW_OUTPUT_FORMAT_PROMPT is empty; Ollama interview JSON reliability may be reduced")
		}
		if cfg.UnstructuredReportOutputFormatPrompt == "" {
			slog.Warn("UNSTRUCTURED_REPORT_OUTPUT_FORMAT_PROMPT is empty; Ollama report JSON reliability may be reduced")
		}
	}

	// Parse AI_AREA_CONFIG JSON array.
	areaJSON := os.Getenv("AI_AREA_CONFIG")
	if areaJSON != "" {
		var areas []AreaConfig
		if err := json.Unmarshal([]byte(areaJSON), &areas); err != nil {
			return Config{}, fmt.Errorf("invalid AI_AREA_CONFIG JSON: %w", err)
		}
		cfg.AreaConfigs = areas
	}

	return cfg, nil
}

// LogLoaded logs all loaded config values at debug level.
// Call this AFTER the slog default handler has been configured.
func (c Config) LogLoaded() {
	slog.Debug("config loaded",
		"port", c.Port,
		"frontend_url", c.FrontendURL,
		"database_url_set", c.DatabaseURL != "",
		"session_expiry_hours", c.SessionExpiryHours,
		"mock_api_url", c.MockAPIURL,
		"log_level", c.LogLevel,
	)
	slog.Debug("AI config loaded",
		"provider", c.AIProvider,
		"ollama_base_url", c.OllamaBaseURL,
		"model", c.AIModel,
		"max_tokens", c.AIMaxTokens,
		"api_key_set", c.AIAPIKey != "",
		"unstructured_interview_output_format_prompt_len", len(c.UnstructuredInterviewOutputFormatPrompt),
		"system_prompt_len", len(c.AISystemPrompt),
		"prompt_last_q_len", len(c.AIPromptLastQ),
		"prompt_closing_len", len(c.AIPromptClosing),
		"last_q_seconds", c.AILastQSeconds,
		"closing_seconds", c.AIClosingSeconds,
		"midpoint_area_index", c.AIMidpointAreaIdx,
		"ai_timeout_seconds", c.AITimeoutSeconds,
		"area_configs_count", len(c.AreaConfigs),
		"report_prompt_len", len(c.AIReportPrompt),
		"unstructured_report_output_format_prompt_len", len(c.UnstructuredReportOutputFormatPrompt),
		"report_max_tokens", c.AIReportMaxTokens,
	)
	for _, ac := range c.AreaConfigs {
		slog.Debug("area config",
			"id", ac.ID,
			"slug", ac.Slug,
			"label", ac.Label,
			"description_len", len(ac.Description),
			"fallback_question_len", len(ac.FallbackQuestion),
		)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
