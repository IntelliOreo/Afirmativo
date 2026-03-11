// Package config defines the application configuration struct,
// loads values from environment variables, and validates them at startup.
// Fails fast on missing or invalid configuration.
package config

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
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
	Port                            string
	FrontendURL                     string
	DatabaseURL                     string
	SessionExpiryHours              int
	JWTSecret                       string
	SessionAuthIssuer               string
	SessionAuthAudience             string
	SessionAuthCookieName           string
	SessionAuthMaxTTLMinutes        int
	MockAPIURL                      string // If non-empty, use this mock server instead of real AI APIs
	LogLevel                        string // "debug", "info", "warn", "error" — defaults to "info"
	AllowSensitiveDebugLogs         bool   // Allows sensitive fields in DEBUG payload logs when true
	HTTPReadTimeoutSeconds          int    // HTTP server read timeout in seconds
	HTTPWriteTimeoutSeconds         int    // HTTP server write timeout in seconds
	HTTPIdleTimeoutSeconds          int    // HTTP server idle timeout in seconds
	AsyncAnswerWorkers              int    // Number of async answer worker goroutines
	AsyncAnswerQueueSize            int    // In-memory queue size for async answer dispatch
	AsyncAnswerRecoveryBatch        int    // Max queued jobs fetched per recovery cycle
	AsyncAnswerRecoveryEverySeconds int    // Recovery loop interval in seconds
	AsyncAnswerStaleAfterSeconds    int    // Running job stale threshold in seconds
	AsyncAnswerJobTimeoutSeconds    int    // Per-job processing timeout in seconds
	AsyncReportWorkers              int    // Number of async report worker goroutines
	AsyncReportQueueSize            int    // In-memory queue size for async report dispatch
	AsyncReportRecoveryBatch        int    // Max queued reports fetched per recovery cycle
	AsyncReportRecoveryEverySeconds int    // Report recovery loop interval in seconds
	AsyncReportStaleAfterSeconds    int    // Running report stale threshold in seconds
	AsyncReportJobTimeoutSeconds    int    // Per-report processing timeout in seconds
	VerifyIPRatePerMinute           int    // Max average /api/session/verify requests per minute per IP
	VerifyIPBurst                   int    // Burst size for /api/session/verify per-IP token bucket
	VerifyFailMaxAttempts           int    // Max verify failures before lockout per session+IP
	VerifyFailWindowSeconds         int    // Window in seconds for counting verify failures
	VerifyFailLockoutSeconds        int    // Lockout duration in seconds after verify failures threshold
	VoiceIPRatePerMinute            int    // Max average /api/voice/token requests per minute per IP
	VoiceIPBurst                    int    // Burst size for /api/voice/token per-IP token bucket
	VoiceSessionRatePerMinute       int    // Max average /api/voice/token requests per minute per session
	VoiceSessionBurst               int    // Burst size for /api/voice/token per-session token bucket

	// AI configuration — all AI instructions live here, not in Go code.
	AIProvider                              string       // "claude" (default) or "ollama"
	OllamaBaseURL                           string       // Base URL for Ollama OpenAI-compatible endpoint
	OllamaTemperature                       float64      // Sampling temperature for Ollama chat completions
	AIInterviewSystemPrompt                 string       // Base interview system prompt sent to Claude/Ollama on every turn
	UnstructuredInterviewOutputFormatPrompt string       // Prompt instructions for unstructured providers to return interview JSON
	AIInterviewPromptLastQuestion           string       // Turn-local priority snippet for last-question pacing
	AIInterviewPromptClosing                string       // Turn-local priority snippet for whole-interview closing
	AIInterviewPromptOpeningTurn            string       // Opening-turn instruction added to the current user turn payload
	AIInterviewLastQuestionSeconds          int          // Time threshold (seconds) to trigger last-question prompt
	AIInterviewClosingSeconds               int          // Time threshold (seconds) to trigger closing prompt
	AIInterviewMidpointAreaIndex            int          // Area index defining the pacing midpoint (e.g. 3 = nexus)
	AIInterviewPromptCachingEnabled         bool         // Enables Claude prompt caching when true
	AIModel                                 string       // e.g. "claude-sonnet-4-20250514"
	AIMaxTokens                             int          // e.g. 1024
	AIAPIKey                                string       // Anthropic API key (not required for Ollama or when MOCK_API_URL is set)
	AITimeoutSeconds                        int          // HTTP timeout for AI API calls (default 30)
	AIReportPrompt                          string       // System prompt for report generation AI call
	UnstructuredReportOutputFormatPrompt    string       // Prompt instructions for unstructured providers to return report JSON
	AIReportMaxTokens                       int          // Max tokens for report AI call (default 2048)
	VertexAIAuthMode                        string       // "api_key" (default) or "adc"
	VertexAIAPIKey                          string       // Google Cloud API key for Vertex API-key auth
	VertexAIProjectID                       string       // GCP project used for Vertex standard endpoints
	VertexAILocation                        string       // Vertex location (default "global")
	VertexAIExplicitCacheEnabled            bool         // Enables explicit cachedContents usage when true
	VertexAIContextCacheTTLSeconds          int          // Explicit cachedContents TTL in seconds
	AreaConfigs                             []AreaConfig // Per-area rubrics loaded from AI_AREA_CONFIG JSON
	InterviewOpeningDisclaimerEn            string       // Opening disclaimer shown on interview/start in English
	InterviewOpeningDisclaimerEs            string       // Opening disclaimer shown on interview/start in Spanish
	InterviewReadinessQuestionEn            string       // Non-criteria readiness question shown after disclaimer in English
	InterviewReadinessQuestionEs            string       // Non-criteria readiness question shown after disclaimer in Spanish

	// Voice AI configuration.
	VoiceAIBaseURL             string // Deepgram-compatible base URL (real or mock)
	VoiceAIAPIKey              string // Voice provider master API key (server only)
	VoiceAIModel               string // Model label returned to the frontend
	VoiceAITokenTimeoutSeconds int    // Minted token TTL in seconds

	// Admin maintenance configuration.
	AdminCleanupEnabled bool // Enables destructive admin cleanup endpoint when true
}

const (
	defaultSessionAuthIssuer   = "afirmativo-backend"
	defaultSessionAuthAudience = "afirmativo-frontend"
)

// Load reads required environment variables and returns a validated Config.
// Returns an error if any required variable is missing.
func Load() (Config, error) {
	expiry, err := envInt("SESSION_EXPIRY_HOURS", 24)
	if err != nil {
		return Config{}, err
	}
	maxTokens, err := envInt("AI_MAX_TOKENS", 1024)
	if err != nil {
		return Config{}, err
	}
	lastQSec, err := envInt("AI_INTERVIEW_LAST_QUESTION_SECONDS", 30)
	if err != nil {
		return Config{}, err
	}
	closingSec, err := envInt("AI_INTERVIEW_CLOSING_SECONDS", 15)
	if err != nil {
		return Config{}, err
	}
	midpointIdx, err := envInt("AI_INTERVIEW_MIDPOINT_AREA_INDEX", 3)
	if err != nil {
		return Config{}, err
	}
	aiTimeout, err := envInt("AI_TIMEOUT_SECONDS", 30)
	if err != nil {
		return Config{}, err
	}
	reportMaxTokens, err := envInt("AI_REPORT_MAX_TOKENS", 2048)
	if err != nil {
		return Config{}, err
	}
	vertexCacheTTLSeconds, err := envIntMin("VERTEX_AI_CONTEXT_CACHE_TTL_SECONDS", 300, 1)
	if err != nil {
		return Config{}, err
	}
	httpReadTimeout, err := envInt("HTTP_READ_TIMEOUT_SECONDS", 10)
	if err != nil {
		return Config{}, err
	}
	httpWriteTimeout, err := envInt("HTTP_WRITE_TIMEOUT_SECONDS", 30)
	if err != nil {
		return Config{}, err
	}
	vertexExplicitCacheEnabled, err := envBool("VERTEX_AI_EXPLICIT_CACHE_ENABLED", true)
	if err != nil {
		return Config{}, err
	}
	httpIdleTimeout, err := envInt("HTTP_IDLE_TIMEOUT_SECONDS", 60)
	if err != nil {
		return Config{}, err
	}
	sessionAuthMaxTTL, err := envIntMin("SESSION_AUTH_MAX_TTL_MINUTES", 60, 1)
	if err != nil {
		return Config{}, err
	}
	ollamaTemp, err := envFloat("OLLAMA_TEMPERATURE", 0.3, 0, 2)
	if err != nil {
		return Config{}, err
	}
	voiceTokenTimeout, err := envInt("VOICE_AI_TOKEN_TIMEOUT_SECONDS", 30)
	if err != nil {
		return Config{}, err
	}
	if voiceTokenTimeout <= 0 || voiceTokenTimeout > 3600 {
		return Config{}, fmt.Errorf("VOICE_AI_TOKEN_TIMEOUT_SECONDS must be between 1 and 3600")
	}
	adminCleanupEnabled, err := envBool("ADMIN_CLEANUP_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	allowSensitiveDebugLogs, err := envBool("ALLOW_SENSITIVE_DEBUG_LOGS", false)
	if err != nil {
		return Config{}, err
	}
	interviewPromptCachingEnabled, err := envBool("AI_INTERVIEW_PROMPT_CACHING_ENABLED", true)
	if err != nil {
		return Config{}, err
	}
	asyncWorkers, err := envIntMin("ASYNC_ANSWER_WORKERS", 4, 1)
	if err != nil {
		return Config{}, err
	}
	asyncQueueSize, err := envIntMin("ASYNC_ANSWER_QUEUE_SIZE", 256, 1)
	if err != nil {
		return Config{}, err
	}
	asyncRecoveryBatch, err := envIntMin("ASYNC_ANSWER_RECOVERY_BATCH", 100, 1)
	if err != nil {
		return Config{}, err
	}
	asyncRecoveryEveryS, err := envIntMin("ASYNC_ANSWER_RECOVERY_EVERY_SECONDS", 10, 1)
	if err != nil {
		return Config{}, err
	}
	asyncStaleAfterS, err := envIntMin("ASYNC_ANSWER_STALE_AFTER_SECONDS", 180, 1)
	if err != nil {
		return Config{}, err
	}
	asyncJobTimeoutS, err := envIntMin("ASYNC_ANSWER_JOB_TIMEOUT_SECONDS", 180, 1)
	if err != nil {
		return Config{}, err
	}
	asyncReportWorkers, err := envIntMin("ASYNC_REPORT_WORKERS", 2, 1)
	if err != nil {
		return Config{}, err
	}
	asyncReportQueueSize, err := envIntMin("ASYNC_REPORT_QUEUE_SIZE", 64, 1)
	if err != nil {
		return Config{}, err
	}
	asyncReportRecoveryBatch, err := envIntMin("ASYNC_REPORT_RECOVERY_BATCH", 50, 1)
	if err != nil {
		return Config{}, err
	}
	asyncReportRecoveryEveryS, err := envIntMin("ASYNC_REPORT_RECOVERY_EVERY_SECONDS", 10, 1)
	if err != nil {
		return Config{}, err
	}
	asyncReportStaleAfterS, err := envIntMin("ASYNC_REPORT_STALE_AFTER_SECONDS", 180, 1)
	if err != nil {
		return Config{}, err
	}
	asyncReportJobTimeoutS, err := envIntMin("ASYNC_REPORT_JOB_TIMEOUT_SECONDS", 180, 1)
	if err != nil {
		return Config{}, err
	}
	verifyIPRatePerMinute, err := envIntMin("VERIFY_IP_RATE_LIMIT_PER_MINUTE", 60, 1)
	if err != nil {
		return Config{}, err
	}
	verifyIPBurst, err := envIntMin("VERIFY_IP_RATE_LIMIT_BURST", 10, 1)
	if err != nil {
		return Config{}, err
	}
	verifyFailMaxAttempts, err := envIntMin("VERIFY_FAIL_MAX_ATTEMPTS", 5, 1)
	if err != nil {
		return Config{}, err
	}
	verifyFailWindowS, err := envIntMin("VERIFY_FAIL_WINDOW_SECONDS", 600, 1)
	if err != nil {
		return Config{}, err
	}
	verifyFailLockoutS, err := envIntMin("VERIFY_FAIL_LOCKOUT_SECONDS", 900, 1)
	if err != nil {
		return Config{}, err
	}
	voiceIPRatePerMinute, err := envIntMin("VOICE_TOKEN_IP_RATE_LIMIT_PER_MINUTE", 30, 1)
	if err != nil {
		return Config{}, err
	}
	voiceIPBurst, err := envIntMin("VOICE_TOKEN_IP_RATE_LIMIT_BURST", 6, 1)
	if err != nil {
		return Config{}, err
	}
	voiceSessionRatePerMin, err := envIntMin("VOICE_TOKEN_SESSION_RATE_LIMIT_PER_MINUTE", 6, 1)
	if err != nil {
		return Config{}, err
	}
	voiceSessionBurst, err := envIntMin("VOICE_TOKEN_SESSION_RATE_LIMIT_BURST", 2, 1)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		Port:                                    envOr("PORT", "8080"),
		FrontendURL:                             envOr("FRONTEND_URL", "http://localhost:3000"),
		DatabaseURL:                             os.Getenv("DATABASE_URL"),
		SessionExpiryHours:                      expiry,
		JWTSecret:                               os.Getenv("JWT_SECRET"),
		SessionAuthIssuer:                       defaultSessionAuthIssuer,
		SessionAuthAudience:                     defaultSessionAuthAudience,
		SessionAuthCookieName:                   envOr("SESSION_AUTH_COOKIE_NAME", "afirmativo_auth"),
		SessionAuthMaxTTLMinutes:                sessionAuthMaxTTL,
		MockAPIURL:                              os.Getenv("MOCK_API_URL"),
		LogLevel:                                envOr("LOG_LEVEL", "info"),
		AllowSensitiveDebugLogs:                 allowSensitiveDebugLogs,
		HTTPReadTimeoutSeconds:                  httpReadTimeout,
		HTTPWriteTimeoutSeconds:                 httpWriteTimeout,
		HTTPIdleTimeoutSeconds:                  httpIdleTimeout,
		AsyncAnswerWorkers:                      asyncWorkers,
		AsyncAnswerQueueSize:                    asyncQueueSize,
		AsyncAnswerRecoveryBatch:                asyncRecoveryBatch,
		AsyncAnswerRecoveryEverySeconds:         asyncRecoveryEveryS,
		AsyncAnswerStaleAfterSeconds:            asyncStaleAfterS,
		AsyncAnswerJobTimeoutSeconds:            asyncJobTimeoutS,
		AsyncReportWorkers:                      asyncReportWorkers,
		AsyncReportQueueSize:                    asyncReportQueueSize,
		AsyncReportRecoveryBatch:                asyncReportRecoveryBatch,
		AsyncReportRecoveryEverySeconds:         asyncReportRecoveryEveryS,
		AsyncReportStaleAfterSeconds:            asyncReportStaleAfterS,
		AsyncReportJobTimeoutSeconds:            asyncReportJobTimeoutS,
		VerifyIPRatePerMinute:                   verifyIPRatePerMinute,
		VerifyIPBurst:                           verifyIPBurst,
		VerifyFailMaxAttempts:                   verifyFailMaxAttempts,
		VerifyFailWindowSeconds:                 verifyFailWindowS,
		VerifyFailLockoutSeconds:                verifyFailLockoutS,
		VoiceIPRatePerMinute:                    voiceIPRatePerMinute,
		VoiceIPBurst:                            voiceIPBurst,
		VoiceSessionRatePerMinute:               voiceSessionRatePerMin,
		VoiceSessionBurst:                       voiceSessionBurst,
		AIProvider:                              envOr("AI_PROVIDER", "claude"),
		OllamaBaseURL:                           envOr("OLLAMA_BASE_URL", "http://localhost:11434"),
		OllamaTemperature:                       ollamaTemp,
		AIInterviewSystemPrompt:                 os.Getenv("AI_INTERVIEW_SYSTEM_PROMPT"),
		UnstructuredInterviewOutputFormatPrompt: os.Getenv("UNSTRUCTURED_INTERVIEW_OUTPUT_FORMAT_PROMPT"),
		AIInterviewPromptLastQuestion:           os.Getenv("AI_INTERVIEW_PROMPT_LAST_QUESTION"),
		AIInterviewPromptClosing:                os.Getenv("AI_INTERVIEW_PROMPT_CLOSING"),
		AIInterviewPromptOpeningTurn:            os.Getenv("AI_INTERVIEW_PROMPT_OPENING_TURN"),
		AIInterviewLastQuestionSeconds:          lastQSec,
		AIInterviewClosingSeconds:               closingSec,
		AIInterviewMidpointAreaIndex:            midpointIdx,
		AIInterviewPromptCachingEnabled:         interviewPromptCachingEnabled,
		AIModel:                                 envOr("AI_MODEL", "claude-sonnet-4-20250514"),
		AIMaxTokens:                             maxTokens,
		AIAPIKey:                                os.Getenv("AI_API_KEY"),
		AITimeoutSeconds:                        aiTimeout,
		AIReportPrompt:                          os.Getenv("AI_REPORT_PROMPT"),
		UnstructuredReportOutputFormatPrompt:    os.Getenv("UNSTRUCTURED_REPORT_OUTPUT_FORMAT_PROMPT"),
		AIReportMaxTokens:                       reportMaxTokens,
		VertexAIAuthMode:                        envOr("VERTEX_AI_AUTH_MODE", "api_key"),
		VertexAIAPIKey:                          os.Getenv("VERTEX_AI_API_KEY"),
		VertexAIProjectID:                       os.Getenv("VERTEX_AI_PROJECT_ID"),
		VertexAILocation:                        envOr("VERTEX_AI_LOCATION", "global"),
		VertexAIExplicitCacheEnabled:            vertexExplicitCacheEnabled,
		VertexAIContextCacheTTLSeconds:          vertexCacheTTLSeconds,
		InterviewOpeningDisclaimerEn:            os.Getenv("INTERVIEW_OPENING_DISCLAIMER_EN"),
		InterviewOpeningDisclaimerEs:            os.Getenv("INTERVIEW_OPENING_DISCLAIMER_ES"),
		InterviewReadinessQuestionEn:            os.Getenv("INTERVIEW_READINESS_QUESTION_EN"),
		InterviewReadinessQuestionEs:            os.Getenv("INTERVIEW_READINESS_QUESTION_ES"),
		VoiceAIBaseURL:                          envOr("VOICE_AI_BASE_URL", envOr("MOCK_API_URL", "https://api.deepgram.com")),
		VoiceAIAPIKey:                           os.Getenv("VOICE_AI_API_KEY"),
		VoiceAIModel:                            envOr("VOICE_AI_MODEL", "nova-3"),
		VoiceAITokenTimeoutSeconds:              voiceTokenTimeout,
		AdminCleanupEnabled:                     adminCleanupEnabled,
	}

	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.JWTSecret == "" {
		return Config{}, fmt.Errorf("JWT_SECRET is required")
	}

	if cfg.AIProvider != "claude" && cfg.AIProvider != "ollama" && cfg.AIProvider != "vertex" {
		return Config{}, fmt.Errorf("invalid AI_PROVIDER %q (expected \"claude\", \"ollama\", or \"vertex\")", cfg.AIProvider)
	}

	// AI_API_KEY is required for Claude unless using mock server.
	if cfg.AIProvider == "claude" && cfg.AIAPIKey == "" && cfg.MockAPIURL == "" {
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
	if cfg.AIProvider == "vertex" {
		cfg.VertexAIAuthMode = strings.TrimSpace(cfg.VertexAIAuthMode)
		if cfg.VertexAIAuthMode != "api_key" && cfg.VertexAIAuthMode != "adc" {
			return Config{}, fmt.Errorf("invalid VERTEX_AI_AUTH_MODE %q (expected \"api_key\" or \"adc\")", cfg.VertexAIAuthMode)
		}
		if strings.TrimSpace(cfg.MockAPIURL) != "" {
			return Config{}, fmt.Errorf("MOCK_API_URL is not supported when AI_PROVIDER=vertex")
		}
		if strings.TrimSpace(cfg.VertexAIProjectID) == "" {
			return Config{}, fmt.Errorf("VERTEX_AI_PROJECT_ID is required when AI_PROVIDER=vertex")
		}
		if cfg.VertexAIAuthMode == "api_key" && strings.TrimSpace(cfg.VertexAIAPIKey) == "" {
			return Config{}, fmt.Errorf("VERTEX_AI_API_KEY is required when AI_PROVIDER=vertex and VERTEX_AI_AUTH_MODE=api_key")
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
		"jwt_secret_set", c.JWTSecret != "",
		"session_auth_issuer", c.SessionAuthIssuer,
		"session_auth_audience", c.SessionAuthAudience,
		"session_auth_cookie_name", c.SessionAuthCookieName,
		"session_auth_max_ttl_minutes", c.SessionAuthMaxTTLMinutes,
		"mock_api_url", c.MockAPIURL,
		"log_level", c.LogLevel,
		"allow_sensitive_debug_logs", c.AllowSensitiveDebugLogs,
		"http_read_timeout_seconds", c.HTTPReadTimeoutSeconds,
		"http_write_timeout_seconds", c.HTTPWriteTimeoutSeconds,
		"http_idle_timeout_seconds", c.HTTPIdleTimeoutSeconds,
		"async_answer_workers", c.AsyncAnswerWorkers,
		"async_answer_queue_size", c.AsyncAnswerQueueSize,
		"async_answer_recovery_batch", c.AsyncAnswerRecoveryBatch,
		"async_answer_recovery_every_seconds", c.AsyncAnswerRecoveryEverySeconds,
		"async_answer_stale_after_seconds", c.AsyncAnswerStaleAfterSeconds,
		"async_answer_job_timeout_seconds", c.AsyncAnswerJobTimeoutSeconds,
		"async_report_workers", c.AsyncReportWorkers,
		"async_report_queue_size", c.AsyncReportQueueSize,
		"async_report_recovery_batch", c.AsyncReportRecoveryBatch,
		"async_report_recovery_every_seconds", c.AsyncReportRecoveryEverySeconds,
		"async_report_stale_after_seconds", c.AsyncReportStaleAfterSeconds,
		"async_report_job_timeout_seconds", c.AsyncReportJobTimeoutSeconds,
		"verify_ip_rate_limit_per_minute", c.VerifyIPRatePerMinute,
		"verify_ip_rate_limit_burst", c.VerifyIPBurst,
		"verify_fail_max_attempts", c.VerifyFailMaxAttempts,
		"verify_fail_window_seconds", c.VerifyFailWindowSeconds,
		"verify_fail_lockout_seconds", c.VerifyFailLockoutSeconds,
		"voice_token_ip_rate_limit_per_minute", c.VoiceIPRatePerMinute,
		"voice_token_ip_rate_limit_burst", c.VoiceIPBurst,
		"voice_token_session_rate_limit_per_minute", c.VoiceSessionRatePerMinute,
		"voice_token_session_rate_limit_burst", c.VoiceSessionBurst,
		"admin_cleanup_enabled", c.AdminCleanupEnabled,
	)
	slog.Debug("AI config loaded",
		"provider", c.AIProvider,
		"ollama_base_url", c.OllamaBaseURL,
		"ollama_temperature", c.OllamaTemperature,
		"model", c.AIModel,
		"max_tokens", c.AIMaxTokens,
		"api_key_set", c.AIAPIKey != "",
		"unstructured_interview_output_format_prompt_len", len(c.UnstructuredInterviewOutputFormatPrompt),
		"interview_system_prompt_len", len(c.AIInterviewSystemPrompt),
		"interview_prompt_last_question_len", len(c.AIInterviewPromptLastQuestion),
		"interview_prompt_closing_len", len(c.AIInterviewPromptClosing),
		"interview_prompt_opening_turn_len", len(c.AIInterviewPromptOpeningTurn),
		"interview_last_question_seconds", c.AIInterviewLastQuestionSeconds,
		"interview_closing_seconds", c.AIInterviewClosingSeconds,
		"interview_midpoint_area_index", c.AIInterviewMidpointAreaIndex,
		"interview_prompt_caching_enabled", c.AIInterviewPromptCachingEnabled,
		"ai_timeout_seconds", c.AITimeoutSeconds,
		"area_configs_count", len(c.AreaConfigs),
		"report_prompt_len", len(c.AIReportPrompt),
		"unstructured_report_output_format_prompt_len", len(c.UnstructuredReportOutputFormatPrompt),
		"report_max_tokens", c.AIReportMaxTokens,
		"vertex_ai_auth_mode", c.VertexAIAuthMode,
		"vertex_ai_project_id", c.VertexAIProjectID,
		"vertex_ai_location", c.VertexAILocation,
		"vertex_ai_explicit_cache_enabled", c.VertexAIExplicitCacheEnabled,
		"vertex_ai_context_cache_ttl_seconds", c.VertexAIContextCacheTTLSeconds,
		"vertex_ai_api_key_set", c.VertexAIAPIKey != "",
		"interview_opening_disclaimer_en_len", len(c.InterviewOpeningDisclaimerEn),
		"interview_opening_disclaimer_es_len", len(c.InterviewOpeningDisclaimerEs),
		"interview_readiness_question_en_len", len(c.InterviewReadinessQuestionEn),
		"interview_readiness_question_es_len", len(c.InterviewReadinessQuestionEs),
	)
	slog.Debug("voice AI config loaded",
		"voice_ai_base_url", c.VoiceAIBaseURL,
		"voice_ai_model", c.VoiceAIModel,
		"voice_ai_api_key_set", c.VoiceAIAPIKey != "",
		"voice_ai_token_timeout_seconds", c.VoiceAITokenTimeoutSeconds,
		"ai_provider", c.AIProvider,
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
