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
	Port                     string
	FrontendURL              string
	DatabaseURL              string
	SessionExpiryHours       int
	JWTSecret                string
	SessionAuthIssuer        string
	SessionAuthAudience      string
	SessionAuthCookieName    string
	SessionAuthMaxTTLMinutes int
	MockAPIURL               string // If non-empty, use this mock server instead of real AI APIs
	LogLevel                 string // "debug", "info", "warn", "error" — defaults to "info"
	AllowSensitiveDebugLogs  bool   // Allows sensitive fields in DEBUG payload logs when true
	HTTPReadTimeoutS         int    // HTTP server read timeout in seconds
	HTTPWriteTimeoutS        int    // HTTP server write timeout in seconds
	HTTPIdleTimeoutS         int    // HTTP server idle timeout in seconds
	AsyncAnswerWorkers       int    // Number of async answer worker goroutines
	AsyncAnswerQueueSize     int    // In-memory queue size for async answer dispatch
	AsyncAnswerRecoveryBatch int    // Max queued jobs fetched per recovery cycle
	AsyncAnswerRecoveryEvery int    // Recovery loop interval in seconds
	AsyncAnswerStaleAfterS   int    // Running job stale threshold in seconds
	AsyncAnswerJobTimeoutS   int    // Per-job processing timeout in seconds
	VerifyIPRatePerMinute    int    // Max average /api/session/verify requests per minute per IP
	VerifyIPBurst            int    // Burst size for /api/session/verify per-IP token bucket
	VerifyFailMaxAttempts    int    // Max verify failures before lockout per session+IP
	VerifyFailWindowS        int    // Window in seconds for counting verify failures
	VerifyFailLockoutS       int    // Lockout duration in seconds after verify failures threshold
	VoiceIPRatePerMinute     int    // Max average /api/deepgram/token requests per minute per IP
	VoiceIPBurst             int    // Burst size for /api/deepgram/token per-IP token bucket
	VoiceSessionRatePerMin   int    // Max average /api/deepgram/token requests per minute per session
	VoiceSessionBurst        int    // Burst size for /api/deepgram/token per-session token bucket

	// AI configuration — all AI instructions live here, not in Go code.
	AIProvider                              string       // "claude" (default) or "ollama"
	OllamaBaseURL                           string       // Base URL for Ollama OpenAI-compatible endpoint
	OllamaTemperature                       float64      // Sampling temperature for Ollama chat completions
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
	ReportStrengthsLabel                    string       // Semantic label for strengths in report prompts/schema (e.g. "areas of clarity")
	ReportWeaknessesLabel                   string       // Semantic label for weaknesses in report prompts/schema (e.g. "areas to develop further")
	UnstructuredReportOutputFormatPrompt    string       // Prompt instructions for unstructured providers to return report JSON
	AIReportMaxTokens                       int          // Max tokens for report AI call (default 2048)
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

	httpReadTimeoutStr := envOr("HTTP_READ_TIMEOUT_SECONDS", "10")
	httpReadTimeout, err8 := strconv.Atoi(httpReadTimeoutStr)
	if err8 != nil {
		return Config{}, fmt.Errorf("invalid HTTP_READ_TIMEOUT_SECONDS: %w", err8)
	}

	httpWriteTimeoutStr := envOr("HTTP_WRITE_TIMEOUT_SECONDS", "30")
	httpWriteTimeout, err9 := strconv.Atoi(httpWriteTimeoutStr)
	if err9 != nil {
		return Config{}, fmt.Errorf("invalid HTTP_WRITE_TIMEOUT_SECONDS: %w", err9)
	}

	httpIdleTimeoutStr := envOr("HTTP_IDLE_TIMEOUT_SECONDS", "60")
	httpIdleTimeout, err10 := strconv.Atoi(httpIdleTimeoutStr)
	if err10 != nil {
		return Config{}, fmt.Errorf("invalid HTTP_IDLE_TIMEOUT_SECONDS: %w", err10)
	}

	sessionAuthMaxTTLStr := envOr("SESSION_AUTH_MAX_TTL_MINUTES", "60")
	sessionAuthMaxTTL, err11 := strconv.Atoi(sessionAuthMaxTTLStr)
	if err11 != nil {
		return Config{}, fmt.Errorf("invalid SESSION_AUTH_MAX_TTL_MINUTES: %w", err11)
	}
	if sessionAuthMaxTTL <= 0 {
		return Config{}, fmt.Errorf("SESSION_AUTH_MAX_TTL_MINUTES must be > 0")
	}

	ollamaTempStr := envOr("OLLAMA_TEMPERATURE", "0.3")
	ollamaTemp, err12 := strconv.ParseFloat(ollamaTempStr, 64)
	if err12 != nil {
		return Config{}, fmt.Errorf("invalid OLLAMA_TEMPERATURE: %w", err12)
	}
	if ollamaTemp < 0 || ollamaTemp > 2 {
		return Config{}, fmt.Errorf("OLLAMA_TEMPERATURE must be between 0 and 2")
	}

	voiceTokenTimeoutStr := envOr("VOICE_AI_TOKEN_TIMEOUT_SECONDS", "30")
	voiceTokenTimeout, err13 := strconv.Atoi(voiceTokenTimeoutStr)
	if err13 != nil {
		return Config{}, fmt.Errorf("invalid VOICE_AI_TOKEN_TIMEOUT_SECONDS: %w", err13)
	}
	if voiceTokenTimeout <= 0 || voiceTokenTimeout > 3600 {
		return Config{}, fmt.Errorf("VOICE_AI_TOKEN_TIMEOUT_SECONDS must be between 1 and 3600")
	}

	adminCleanupEnabledStr := envOr("ADMIN_CLEANUP_ENABLED", "false")
	adminCleanupEnabled, err14 := strconv.ParseBool(adminCleanupEnabledStr)
	if err14 != nil {
		return Config{}, fmt.Errorf("invalid ADMIN_CLEANUP_ENABLED: %w", err14)
	}

	allowSensitiveDebugLogsStr := envOr("ALLOW_SENSITIVE_DEBUG_LOGS", "false")
	allowSensitiveDebugLogs, err15 := strconv.ParseBool(allowSensitiveDebugLogsStr)
	if err15 != nil {
		return Config{}, fmt.Errorf("invalid ALLOW_SENSITIVE_DEBUG_LOGS: %w", err15)
	}

	asyncWorkers, err := strconv.Atoi(envOr("ASYNC_ANSWER_WORKERS", "4"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid ASYNC_ANSWER_WORKERS: %w", err)
	}
	if asyncWorkers <= 0 {
		return Config{}, fmt.Errorf("ASYNC_ANSWER_WORKERS must be > 0")
	}

	asyncQueueSize, err := strconv.Atoi(envOr("ASYNC_ANSWER_QUEUE_SIZE", "256"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid ASYNC_ANSWER_QUEUE_SIZE: %w", err)
	}
	if asyncQueueSize <= 0 {
		return Config{}, fmt.Errorf("ASYNC_ANSWER_QUEUE_SIZE must be > 0")
	}

	asyncRecoveryBatch, err := strconv.Atoi(envOr("ASYNC_ANSWER_RECOVERY_BATCH", "100"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid ASYNC_ANSWER_RECOVERY_BATCH: %w", err)
	}
	if asyncRecoveryBatch <= 0 {
		return Config{}, fmt.Errorf("ASYNC_ANSWER_RECOVERY_BATCH must be > 0")
	}

	asyncRecoveryEveryS, err := strconv.Atoi(envOr("ASYNC_ANSWER_RECOVERY_EVERY_SECONDS", "10"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid ASYNC_ANSWER_RECOVERY_EVERY_SECONDS: %w", err)
	}
	if asyncRecoveryEveryS <= 0 {
		return Config{}, fmt.Errorf("ASYNC_ANSWER_RECOVERY_EVERY_SECONDS must be > 0")
	}

	asyncStaleAfterS, err := strconv.Atoi(envOr("ASYNC_ANSWER_STALE_AFTER_SECONDS", "180"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid ASYNC_ANSWER_STALE_AFTER_SECONDS: %w", err)
	}
	if asyncStaleAfterS <= 0 {
		return Config{}, fmt.Errorf("ASYNC_ANSWER_STALE_AFTER_SECONDS must be > 0")
	}

	asyncJobTimeoutS, err := strconv.Atoi(envOr("ASYNC_ANSWER_JOB_TIMEOUT_SECONDS", "180"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid ASYNC_ANSWER_JOB_TIMEOUT_SECONDS: %w", err)
	}
	if asyncJobTimeoutS <= 0 {
		return Config{}, fmt.Errorf("ASYNC_ANSWER_JOB_TIMEOUT_SECONDS must be > 0")
	}

	verifyIPRatePerMinute, err := strconv.Atoi(envOr("VERIFY_IP_RATE_LIMIT_PER_MINUTE", "60"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid VERIFY_IP_RATE_LIMIT_PER_MINUTE: %w", err)
	}
	if verifyIPRatePerMinute <= 0 {
		return Config{}, fmt.Errorf("VERIFY_IP_RATE_LIMIT_PER_MINUTE must be > 0")
	}

	verifyIPBurst, err := strconv.Atoi(envOr("VERIFY_IP_RATE_LIMIT_BURST", "10"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid VERIFY_IP_RATE_LIMIT_BURST: %w", err)
	}
	if verifyIPBurst <= 0 {
		return Config{}, fmt.Errorf("VERIFY_IP_RATE_LIMIT_BURST must be > 0")
	}

	verifyFailMaxAttempts, err := strconv.Atoi(envOr("VERIFY_FAIL_MAX_ATTEMPTS", "5"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid VERIFY_FAIL_MAX_ATTEMPTS: %w", err)
	}
	if verifyFailMaxAttempts <= 0 {
		return Config{}, fmt.Errorf("VERIFY_FAIL_MAX_ATTEMPTS must be > 0")
	}

	verifyFailWindowS, err := strconv.Atoi(envOr("VERIFY_FAIL_WINDOW_SECONDS", "600"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid VERIFY_FAIL_WINDOW_SECONDS: %w", err)
	}
	if verifyFailWindowS <= 0 {
		return Config{}, fmt.Errorf("VERIFY_FAIL_WINDOW_SECONDS must be > 0")
	}

	verifyFailLockoutS, err := strconv.Atoi(envOr("VERIFY_FAIL_LOCKOUT_SECONDS", "900"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid VERIFY_FAIL_LOCKOUT_SECONDS: %w", err)
	}
	if verifyFailLockoutS <= 0 {
		return Config{}, fmt.Errorf("VERIFY_FAIL_LOCKOUT_SECONDS must be > 0")
	}

	voiceIPRatePerMinute, err := strconv.Atoi(envOr("VOICE_TOKEN_IP_RATE_LIMIT_PER_MINUTE", "30"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid VOICE_TOKEN_IP_RATE_LIMIT_PER_MINUTE: %w", err)
	}
	if voiceIPRatePerMinute <= 0 {
		return Config{}, fmt.Errorf("VOICE_TOKEN_IP_RATE_LIMIT_PER_MINUTE must be > 0")
	}

	voiceIPBurst, err := strconv.Atoi(envOr("VOICE_TOKEN_IP_RATE_LIMIT_BURST", "6"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid VOICE_TOKEN_IP_RATE_LIMIT_BURST: %w", err)
	}
	if voiceIPBurst <= 0 {
		return Config{}, fmt.Errorf("VOICE_TOKEN_IP_RATE_LIMIT_BURST must be > 0")
	}

	voiceSessionRatePerMin, err := strconv.Atoi(envOr("VOICE_TOKEN_SESSION_RATE_LIMIT_PER_MINUTE", "6"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid VOICE_TOKEN_SESSION_RATE_LIMIT_PER_MINUTE: %w", err)
	}
	if voiceSessionRatePerMin <= 0 {
		return Config{}, fmt.Errorf("VOICE_TOKEN_SESSION_RATE_LIMIT_PER_MINUTE must be > 0")
	}

	voiceSessionBurst, err := strconv.Atoi(envOr("VOICE_TOKEN_SESSION_RATE_LIMIT_BURST", "2"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid VOICE_TOKEN_SESSION_RATE_LIMIT_BURST: %w", err)
	}
	if voiceSessionBurst <= 0 {
		return Config{}, fmt.Errorf("VOICE_TOKEN_SESSION_RATE_LIMIT_BURST must be > 0")
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
		HTTPReadTimeoutS:                        httpReadTimeout,
		HTTPWriteTimeoutS:                       httpWriteTimeout,
		HTTPIdleTimeoutS:                        httpIdleTimeout,
		AsyncAnswerWorkers:                      asyncWorkers,
		AsyncAnswerQueueSize:                    asyncQueueSize,
		AsyncAnswerRecoveryBatch:                asyncRecoveryBatch,
		AsyncAnswerRecoveryEvery:                asyncRecoveryEveryS,
		AsyncAnswerStaleAfterS:                  asyncStaleAfterS,
		AsyncAnswerJobTimeoutS:                  asyncJobTimeoutS,
		VerifyIPRatePerMinute:                   verifyIPRatePerMinute,
		VerifyIPBurst:                           verifyIPBurst,
		VerifyFailMaxAttempts:                   verifyFailMaxAttempts,
		VerifyFailWindowS:                       verifyFailWindowS,
		VerifyFailLockoutS:                      verifyFailLockoutS,
		VoiceIPRatePerMinute:                    voiceIPRatePerMinute,
		VoiceIPBurst:                            voiceIPBurst,
		VoiceSessionRatePerMin:                  voiceSessionRatePerMin,
		VoiceSessionBurst:                       voiceSessionBurst,
		AIProvider:                              envOr("AI_PROVIDER", "claude"),
		OllamaBaseURL:                           envOr("OLLAMA_BASE_URL", "http://localhost:11434"),
		OllamaTemperature:                       ollamaTemp,
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
		ReportStrengthsLabel:                    envOr("REPORT_STRENGTHS_LABEL", "areas-of-clarity"),
		ReportWeaknessesLabel:                   envOr("REPORT_WEAKNESSES_LABEL", "areas-to-develop-further"),
		UnstructuredReportOutputFormatPrompt:    os.Getenv("UNSTRUCTURED_REPORT_OUTPUT_FORMAT_PROMPT"),
		AIReportMaxTokens:                       reportMaxTokens,
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
		"jwt_secret_set", c.JWTSecret != "",
		"session_auth_issuer", c.SessionAuthIssuer,
		"session_auth_audience", c.SessionAuthAudience,
		"session_auth_cookie_name", c.SessionAuthCookieName,
		"session_auth_max_ttl_minutes", c.SessionAuthMaxTTLMinutes,
		"mock_api_url", c.MockAPIURL,
		"log_level", c.LogLevel,
		"allow_sensitive_debug_logs", c.AllowSensitiveDebugLogs,
		"http_read_timeout_seconds", c.HTTPReadTimeoutS,
		"http_write_timeout_seconds", c.HTTPWriteTimeoutS,
		"http_idle_timeout_seconds", c.HTTPIdleTimeoutS,
		"async_answer_workers", c.AsyncAnswerWorkers,
		"async_answer_queue_size", c.AsyncAnswerQueueSize,
		"async_answer_recovery_batch", c.AsyncAnswerRecoveryBatch,
		"async_answer_recovery_every_seconds", c.AsyncAnswerRecoveryEvery,
		"async_answer_stale_after_seconds", c.AsyncAnswerStaleAfterS,
		"async_answer_job_timeout_seconds", c.AsyncAnswerJobTimeoutS,
		"verify_ip_rate_limit_per_minute", c.VerifyIPRatePerMinute,
		"verify_ip_rate_limit_burst", c.VerifyIPBurst,
		"verify_fail_max_attempts", c.VerifyFailMaxAttempts,
		"verify_fail_window_seconds", c.VerifyFailWindowS,
		"verify_fail_lockout_seconds", c.VerifyFailLockoutS,
		"voice_token_ip_rate_limit_per_minute", c.VoiceIPRatePerMinute,
		"voice_token_ip_rate_limit_burst", c.VoiceIPBurst,
		"voice_token_session_rate_limit_per_minute", c.VoiceSessionRatePerMin,
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
		"system_prompt_len", len(c.AISystemPrompt),
		"prompt_last_q_len", len(c.AIPromptLastQ),
		"prompt_closing_len", len(c.AIPromptClosing),
		"last_q_seconds", c.AILastQSeconds,
		"closing_seconds", c.AIClosingSeconds,
		"midpoint_area_index", c.AIMidpointAreaIdx,
		"ai_timeout_seconds", c.AITimeoutSeconds,
		"area_configs_count", len(c.AreaConfigs),
		"report_prompt_len", len(c.AIReportPrompt),
		"report_strengths_label", c.ReportStrengthsLabel,
		"report_weaknesses_label", c.ReportWeaknessesLabel,
		"unstructured_report_output_format_prompt_len", len(c.UnstructuredReportOutputFormatPrompt),
		"report_max_tokens", c.AIReportMaxTokens,
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
