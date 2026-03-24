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
	"time"
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

type BilingualText struct {
	En string
	Es string
}

type AsyncRuntimeConfig struct {
	Workers       int
	QueueSize     int
	RecoveryEvery time.Duration
	StaleAfter    time.Duration
	JobTimeout    time.Duration
}

type ServerConfig struct {
	Port                    string
	FrontendURL             string
	DatabaseURL             string
	LogLevel                string
	LogFormat               string // "text" (default, human-readable) or "json" (GCP Cloud Logging)
	AllowSensitiveDebugLogs bool
	HTTPReadTimeout         time.Duration
	HTTPWriteTimeout        time.Duration
	HTTPIdleTimeout         time.Duration
}

type AuthConfig struct {
	JWTSecret             string
	SessionExpiryHours    int
	SessionAuthIssuer     string
	SessionAuthAudience   string
	SessionAuthCookieName string
	SessionAuthMaxTTL     time.Duration
}

type InterviewConfig struct {
	BudgetSeconds          int
	AnswerTimeLimitSeconds int
	AsyncRuntime           AsyncRuntimeConfig
	AreaConfigs            []AreaConfig
	OpeningDisclaimer      BilingualText
	ReadinessQuestion      BilingualText
}

type ReportConfig struct {
	AsyncRuntime AsyncRuntimeConfig
}

type AIConfig struct {
	Provider                                string
	MockAPIURL                              string
	OllamaBaseURL                           string
	OllamaTemperature                       float64
	InterviewSystemPrompt                   string
	UnstructuredInterviewOutputFormatPrompt string
	InterviewPromptLastQuestion             string
	InterviewPromptClosing                  string
	InterviewPromptOpeningTurn              string
	InterviewLastQuestionSeconds            int
	InterviewClosingSeconds                 int
	InterviewMidpointAreaIndex              int
	InterviewPromptCachingEnabled           bool
	Model                                   string
	MaxTokens                               int
	APIKey                                  string
	Timeout                                 time.Duration
	ReportPrompt                            string
	UnstructuredReportOutputFormatPrompt    string
	ReportMaxTokens                         int
	VertexAuthMode                          string
	VertexAPIKey                            string
	VertexProjectID                         string
	VertexLocation                          string
	VertexExplicitCacheEnabled              bool
	VertexContextCacheTTL                   time.Duration
}

type VoiceConfig struct {
	BaseURL             string
	APIKey              string
	Model               string
	TokenTimeoutSeconds int
	Timeout             time.Duration
}

type PaymentConfig struct {
	StripeSecretKey          string
	StripeWebhookSecret      string
	DirectSessionAmountCents int
	CouponPack10AmountCents  int
}

type VerifyRateLimitConfig struct {
	IPRatePerMinute int
	IPBurst         int
	FailMaxAttempts int
	FailWindow      time.Duration
	FailLockout     time.Duration
}

type VoiceRateLimitConfig struct {
	IPRatePerMinute      int
	IPBurst              int
	SessionRatePerMinute int
	SessionBurst         int
}

type RateLimitConfig struct {
	Verify VerifyRateLimitConfig
	Voice  VoiceRateLimitConfig
}

type OTelConfig struct {
	Enabled      bool
	GCPProjectID string
}

type AdminConfig struct {
	CleanupEnabled bool
}

// Config holds all application configuration loaded from environment variables.
// In local dev, values come from .env loaded by the IDE/debugger (e.g. VS Code envFile).
// In containers, values come from the runtime environment (e.g., Secret Manager).
type Config struct {
	Server             ServerConfig
	Auth               AuthConfig
	DBOperationTimeout time.Duration
	Interview          InterviewConfig
	Report             ReportConfig
	AI                 AIConfig
	Voice              VoiceConfig
	Payment            PaymentConfig
	RateLimit          RateLimitConfig
	OTel               OTelConfig
	Admin              AdminConfig
}

const (
	defaultSessionAuthIssuer   = "afirmativo-backend"
	defaultSessionAuthAudience = "afirmativo-frontend"
	defaultDBOperationTimeout  = 5 * time.Second

	defaultOpeningDisclaimerEs = "Aviso importante: esta entrevista simulada es solo para preparacion y no constituye asesoramiento legal. Al continuar, usted confirma que leyo y acepta estos terminos."
	defaultOpeningDisclaimerEn = "Important disclaimer: this mock interview is for preparation only and does not constitute legal advice. By continuing, you confirm that you read and accept these terms."
	defaultReadinessQuestionEs = "¿Cómo se siente hoy? ¿Está física y mentalmente preparado/a para continuar con esta entrevista?"
	defaultReadinessQuestionEn = "How are you feeling today? Are you physically and mentally ready to proceed with this interview?"
)

// Load reads required environment variables and returns a validated Config.
// Returns an error if any required variable is missing.
func Load() (Config, error) {
	sessionExpiryHours, err := envInt("SESSION_EXPIRY_HOURS", 24)
	if err != nil {
		return Config{}, err
	}
	interviewBudgetSeconds, err := envIntMin("INTERVIEW_BUDGET_SECONDS", 2400, 1)
	if err != nil {
		return Config{}, err
	}
	answerTimeLimitSeconds, err := envIntMin("ANSWER_TIME_LIMIT_SECONDS", 300, 30)
	if err != nil {
		return Config{}, err
	}
	sessionAuthMaxTTLMinutes, err := envIntMin("SESSION_AUTH_MAX_TTL_MINUTES", 60, 1)
	if err != nil {
		return Config{}, err
	}
	maxTokens, err := envInt("AI_MAX_TOKENS", 1024)
	if err != nil {
		return Config{}, err
	}
	reportMaxTokens, err := envInt("AI_REPORT_MAX_TOKENS", 2048)
	if err != nil {
		return Config{}, err
	}
	lastQuestionSeconds, err := envInt("AI_INTERVIEW_LAST_QUESTION_SECONDS", 30)
	if err != nil {
		return Config{}, err
	}
	closingSeconds, err := envInt("AI_INTERVIEW_CLOSING_SECONDS", 15)
	if err != nil {
		return Config{}, err
	}
	midpointAreaIndex, err := envInt("AI_INTERVIEW_MIDPOINT_AREA_INDEX", 3)
	if err != nil {
		return Config{}, err
	}
	aiTimeoutSeconds, err := envInt("AI_TIMEOUT_SECONDS", 30)
	if err != nil {
		return Config{}, err
	}
	httpReadTimeoutSeconds, err := envInt("HTTP_READ_TIMEOUT_SECONDS", 10)
	if err != nil {
		return Config{}, err
	}
	httpWriteTimeoutSeconds, err := envInt("HTTP_WRITE_TIMEOUT_SECONDS", 30)
	if err != nil {
		return Config{}, err
	}
	httpIdleTimeoutSeconds, err := envInt("HTTP_IDLE_TIMEOUT_SECONDS", 60)
	if err != nil {
		return Config{}, err
	}
	vertexContextCacheTTLSeconds, err := envIntMin("VERTEX_AI_CONTEXT_CACHE_TTL_SECONDS", 300, 1)
	if err != nil {
		return Config{}, err
	}
	vertexExplicitCacheEnabled, err := envBool("VERTEX_AI_EXPLICIT_CACHE_ENABLED", true)
	if err != nil {
		return Config{}, err
	}
	ollamaTemperature, err := envFloat("OLLAMA_TEMPERATURE", 0.3, 0, 2)
	if err != nil {
		return Config{}, err
	}
	voiceTokenTimeoutSeconds, err := envInt("VOICE_AI_TOKEN_TIMEOUT_SECONDS", 30)
	if err != nil {
		return Config{}, err
	}
	paymentAmountCents, err := envIntMin("PAYMENT_AMOUNT_CENTS", 499, 1)
	if err != nil {
		return Config{}, err
	}
	couponPack10AmountCents, err := envIntMin("PAYMENT_COUPON_PACK_10_AMOUNT_CENTS", 3500, 1)
	if err != nil {
		return Config{}, err
	}
	if voiceTokenTimeoutSeconds <= 0 || voiceTokenTimeoutSeconds > 3600 {
		return Config{}, fmt.Errorf("VOICE_AI_TOKEN_TIMEOUT_SECONDS must be between 1 and 3600")
	}
	allowSensitiveDebugLogs, err := envBool("ALLOW_SENSITIVE_DEBUG_LOGS", false)
	if err != nil {
		return Config{}, err
	}
	otelEnabled, err := envBool("OTEL_ENABLED", false)
	if err != nil {
		return Config{}, err
	}

	adminCleanupEnabled, err := envBool("ADMIN_CLEANUP_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	interviewPromptCachingEnabled, err := envBool("AI_INTERVIEW_PROMPT_CACHING_ENABLED", true)
	if err != nil {
		return Config{}, err
	}

	answerAsyncWorkers, err := envIntMin("ASYNC_ANSWER_WORKERS", 4, 1)
	if err != nil {
		return Config{}, err
	}
	answerAsyncQueueSize, err := envIntMin("ASYNC_ANSWER_QUEUE_SIZE", 256, 1)
	if err != nil {
		return Config{}, err
	}
	answerAsyncRecoveryEverySeconds, err := envIntMin("ASYNC_ANSWER_RECOVERY_EVERY_SECONDS", 10, 1)
	if err != nil {
		return Config{}, err
	}
	answerAsyncStaleAfterSeconds, err := envIntMin("ASYNC_ANSWER_STALE_AFTER_SECONDS", 180, 1)
	if err != nil {
		return Config{}, err
	}
	answerAsyncJobTimeoutSeconds, err := envIntMin("ASYNC_ANSWER_JOB_TIMEOUT_SECONDS", 180, 1)
	if err != nil {
		return Config{}, err
	}

	reportAsyncWorkers, err := envIntMin("ASYNC_REPORT_WORKERS", 2, 1)
	if err != nil {
		return Config{}, err
	}
	reportAsyncQueueSize, err := envIntMin("ASYNC_REPORT_QUEUE_SIZE", 64, 1)
	if err != nil {
		return Config{}, err
	}
	reportAsyncRecoveryEverySeconds, err := envIntMin("ASYNC_REPORT_RECOVERY_EVERY_SECONDS", 10, 1)
	if err != nil {
		return Config{}, err
	}
	reportAsyncStaleAfterSeconds, err := envIntMin("ASYNC_REPORT_STALE_AFTER_SECONDS", 180, 1)
	if err != nil {
		return Config{}, err
	}
	reportAsyncJobTimeoutSeconds, err := envIntMin("ASYNC_REPORT_JOB_TIMEOUT_SECONDS", 180, 1)
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
	verifyFailWindowSeconds, err := envIntMin("VERIFY_FAIL_WINDOW_SECONDS", 600, 1)
	if err != nil {
		return Config{}, err
	}
	verifyFailLockoutSeconds, err := envIntMin("VERIFY_FAIL_LOCKOUT_SECONDS", 900, 1)
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
	voiceSessionRatePerMinute, err := envIntMin("VOICE_TOKEN_SESSION_RATE_LIMIT_PER_MINUTE", 6, 1)
	if err != nil {
		return Config{}, err
	}
	voiceSessionBurst, err := envIntMin("VOICE_TOKEN_SESSION_RATE_LIMIT_BURST", 2, 1)
	if err != nil {
		return Config{}, err
	}

	areaConfigs, err := parseAreaConfigs(os.Getenv("AI_AREA_CONFIG"))
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		Server: ServerConfig{
			Port:                    envOr("PORT", "8080"),
			FrontendURL:             envOr("FRONTEND_URL", "http://localhost:3000"),
			DatabaseURL:             os.Getenv("DATABASE_URL"),
			LogLevel:                envOr("LOG_LEVEL", "info"),
			LogFormat:               envOr("LOG_FORMAT", "text"),
			AllowSensitiveDebugLogs: allowSensitiveDebugLogs,
			HTTPReadTimeout:         time.Duration(httpReadTimeoutSeconds) * time.Second,
			HTTPWriteTimeout:        time.Duration(httpWriteTimeoutSeconds) * time.Second,
			HTTPIdleTimeout:         time.Duration(httpIdleTimeoutSeconds) * time.Second,
		},
		Auth: AuthConfig{
			JWTSecret:             os.Getenv("JWT_SECRET"),
			SessionExpiryHours:    sessionExpiryHours,
			SessionAuthIssuer:     defaultSessionAuthIssuer,
			SessionAuthAudience:   defaultSessionAuthAudience,
			SessionAuthCookieName: envOr("SESSION_AUTH_COOKIE_NAME", "afirmativo_auth"),
			SessionAuthMaxTTL:     time.Duration(sessionAuthMaxTTLMinutes) * time.Minute,
		},
		DBOperationTimeout: defaultDBOperationTimeout,
		Interview: InterviewConfig{
			BudgetSeconds:          interviewBudgetSeconds,
			AnswerTimeLimitSeconds: answerTimeLimitSeconds,
			AsyncRuntime: AsyncRuntimeConfig{
				Workers:       answerAsyncWorkers,
				QueueSize:     answerAsyncQueueSize,
				RecoveryEvery: time.Duration(answerAsyncRecoveryEverySeconds) * time.Second,
				StaleAfter:    time.Duration(answerAsyncStaleAfterSeconds) * time.Second,
				JobTimeout:    time.Duration(answerAsyncJobTimeoutSeconds) * time.Second,
			},
			AreaConfigs: areaConfigs,
			OpeningDisclaimer: BilingualText{
				En: firstNonEmpty(strings.TrimSpace(os.Getenv("INTERVIEW_OPENING_DISCLAIMER_EN")), defaultOpeningDisclaimerEn),
				Es: firstNonEmpty(strings.TrimSpace(os.Getenv("INTERVIEW_OPENING_DISCLAIMER_ES")), defaultOpeningDisclaimerEs),
			},
			ReadinessQuestion: BilingualText{
				En: firstNonEmpty(strings.TrimSpace(os.Getenv("INTERVIEW_READINESS_QUESTION_EN")), defaultReadinessQuestionEn),
				Es: firstNonEmpty(strings.TrimSpace(os.Getenv("INTERVIEW_READINESS_QUESTION_ES")), defaultReadinessQuestionEs),
			},
		},
		Report: ReportConfig{
			AsyncRuntime: AsyncRuntimeConfig{
				Workers:       reportAsyncWorkers,
				QueueSize:     reportAsyncQueueSize,
				RecoveryEvery: time.Duration(reportAsyncRecoveryEverySeconds) * time.Second,
				StaleAfter:    time.Duration(reportAsyncStaleAfterSeconds) * time.Second,
				JobTimeout:    time.Duration(reportAsyncJobTimeoutSeconds) * time.Second,
			},
		},
		AI: AIConfig{
			Provider:                                envOr("AI_PROVIDER", "claude"),
			MockAPIURL:                              os.Getenv("MOCK_API_URL"),
			OllamaBaseURL:                           envOr("OLLAMA_BASE_URL", "http://localhost:11434"),
			OllamaTemperature:                       ollamaTemperature,
			InterviewSystemPrompt:                   os.Getenv("AI_INTERVIEW_SYSTEM_PROMPT"),
			UnstructuredInterviewOutputFormatPrompt: os.Getenv("UNSTRUCTURED_INTERVIEW_OUTPUT_FORMAT_PROMPT"),
			InterviewPromptLastQuestion:             os.Getenv("AI_INTERVIEW_PROMPT_LAST_QUESTION"),
			InterviewPromptClosing:                  os.Getenv("AI_INTERVIEW_PROMPT_CLOSING"),
			InterviewPromptOpeningTurn:              os.Getenv("AI_INTERVIEW_PROMPT_OPENING_TURN"),
			InterviewLastQuestionSeconds:            lastQuestionSeconds,
			InterviewClosingSeconds:                 closingSeconds,
			InterviewMidpointAreaIndex:              midpointAreaIndex,
			InterviewPromptCachingEnabled:           interviewPromptCachingEnabled,
			Model:                                   envOr("AI_MODEL", "claude-sonnet-4-20250514"),
			MaxTokens:                               maxTokens,
			APIKey:                                  os.Getenv("AI_API_KEY"),
			Timeout:                                 time.Duration(aiTimeoutSeconds) * time.Second,
			ReportPrompt:                            os.Getenv("AI_REPORT_PROMPT"),
			UnstructuredReportOutputFormatPrompt:    os.Getenv("UNSTRUCTURED_REPORT_OUTPUT_FORMAT_PROMPT"),
			ReportMaxTokens:                         reportMaxTokens,
			VertexAuthMode:                          envOr("VERTEX_AI_AUTH_MODE", "api_key"),
			VertexAPIKey:                            os.Getenv("VERTEX_AI_API_KEY"),
			VertexProjectID:                         os.Getenv("VERTEX_AI_PROJECT_ID"),
			VertexLocation:                          envOr("VERTEX_AI_LOCATION", "global"),
			VertexExplicitCacheEnabled:              vertexExplicitCacheEnabled,
			VertexContextCacheTTL:                   time.Duration(vertexContextCacheTTLSeconds) * time.Second,
		},
		Voice: VoiceConfig{
			BaseURL:             envOr("VOICE_AI_BASE_URL", envOr("MOCK_API_URL", "https://api.deepgram.com")),
			APIKey:              os.Getenv("VOICE_AI_API_KEY"),
			Model:               envOr("VOICE_AI_MODEL", "nova-3"),
			TokenTimeoutSeconds: voiceTokenTimeoutSeconds,
			Timeout:             time.Duration(aiTimeoutSeconds) * time.Second,
		},
		Payment: PaymentConfig{
			StripeSecretKey:          os.Getenv("STRIPE_SECRET_KEY"),
			StripeWebhookSecret:      os.Getenv("STRIPE_WEBHOOK_SECRET"),
			DirectSessionAmountCents: paymentAmountCents,
			CouponPack10AmountCents:  couponPack10AmountCents,
		},
		RateLimit: RateLimitConfig{
			Verify: VerifyRateLimitConfig{
				IPRatePerMinute: verifyIPRatePerMinute,
				IPBurst:         verifyIPBurst,
				FailMaxAttempts: verifyFailMaxAttempts,
				FailWindow:      time.Duration(verifyFailWindowSeconds) * time.Second,
				FailLockout:     time.Duration(verifyFailLockoutSeconds) * time.Second,
			},
			Voice: VoiceRateLimitConfig{
				IPRatePerMinute:      voiceIPRatePerMinute,
				IPBurst:              voiceIPBurst,
				SessionRatePerMinute: voiceSessionRatePerMinute,
				SessionBurst:         voiceSessionBurst,
			},
		},
		OTel: OTelConfig{
			Enabled:      otelEnabled,
			GCPProjectID: os.Getenv("GCP_PROJECT_ID"),
		},
		Admin: AdminConfig{
			CleanupEnabled: adminCleanupEnabled,
		},
	}

	if err := validateConfig(cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// LogLoaded logs all loaded config values at debug level.
// Call this AFTER the slog default handler has been configured.
func (c Config) LogLoaded() {
	slog.Debug("config loaded",
		"port", c.Server.Port,
		"frontend_url", c.Server.FrontendURL,
		"database_url_set", c.Server.DatabaseURL != "",
		"log_level", c.Server.LogLevel,
		"allow_sensitive_debug_logs", c.Server.AllowSensitiveDebugLogs,
		"session_expiry_hours", c.Auth.SessionExpiryHours,
		"interview_budget_seconds", c.Interview.BudgetSeconds,
		"answer_time_limit_seconds", c.Interview.AnswerTimeLimitSeconds,
		"db_operation_timeout_seconds", int(c.DBOperationTimeout/time.Second),
		"session_auth_cookie_name", c.Auth.SessionAuthCookieName,
		"session_auth_max_ttl_minutes", int(c.Auth.SessionAuthMaxTTL/time.Minute),
		"admin_cleanup_enabled", c.Admin.CleanupEnabled,
	)
	slog.Debug("async runtime config loaded",
		"interview_async_workers", c.Interview.AsyncRuntime.Workers,
		"interview_async_queue_size", c.Interview.AsyncRuntime.QueueSize,
		"interview_async_recovery_every_seconds", int(c.Interview.AsyncRuntime.RecoveryEvery/time.Second),
		"interview_async_stale_after_seconds", int(c.Interview.AsyncRuntime.StaleAfter/time.Second),
		"interview_async_job_timeout_seconds", int(c.Interview.AsyncRuntime.JobTimeout/time.Second),
		"report_async_workers", c.Report.AsyncRuntime.Workers,
		"report_async_queue_size", c.Report.AsyncRuntime.QueueSize,
		"report_async_recovery_every_seconds", int(c.Report.AsyncRuntime.RecoveryEvery/time.Second),
		"report_async_stale_after_seconds", int(c.Report.AsyncRuntime.StaleAfter/time.Second),
		"report_async_job_timeout_seconds", int(c.Report.AsyncRuntime.JobTimeout/time.Second),
	)
	slog.Debug("AI config loaded",
		"provider", c.AI.Provider,
		"model", c.AI.Model,
		"ollama_base_url", c.AI.OllamaBaseURL,
		"ollama_temperature", c.AI.OllamaTemperature,
		"ai_timeout_seconds", int(c.AI.Timeout/time.Second),
		"max_tokens", c.AI.MaxTokens,
		"report_max_tokens", c.AI.ReportMaxTokens,
		"api_key_set", c.AI.APIKey != "",
		"vertex_ai_auth_mode", c.AI.VertexAuthMode,
		"vertex_ai_project_id", c.AI.VertexProjectID,
		"vertex_ai_location", c.AI.VertexLocation,
		"vertex_ai_context_cache_ttl_seconds", int(c.AI.VertexContextCacheTTL/time.Second),
		"vertex_ai_api_key_set", c.AI.VertexAPIKey != "",
		"area_configs_count", len(c.Interview.AreaConfigs),
	)
	slog.Debug("voice config loaded",
		"voice_ai_base_url", c.Voice.BaseURL,
		"voice_ai_model", c.Voice.Model,
		"voice_ai_api_key_set", c.Voice.APIKey != "",
		"voice_ai_token_timeout_seconds", c.Voice.TokenTimeoutSeconds,
	)
	slog.Debug("payment config loaded",
		"stripe_secret_key_set", c.Payment.StripeSecretKey != "",
		"stripe_webhook_secret_set", c.Payment.StripeWebhookSecret != "",
		"direct_session_amount_cents", c.Payment.DirectSessionAmountCents,
		"coupon_pack_10_amount_cents", c.Payment.CouponPack10AmountCents,
	)
	for _, ac := range c.Interview.AreaConfigs {
		slog.Debug("area config",
			"id", ac.ID,
			"slug", ac.Slug,
			"label", ac.Label,
			"description_len", len(ac.Description),
			"fallback_question_len", len(ac.FallbackQuestion),
		)
	}
}

func validateConfig(cfg Config) error {
	if cfg.Server.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.Auth.JWTSecret == "" {
		return fmt.Errorf("JWT_SECRET is required")
	}
	if cfg.DBOperationTimeout <= 0 {
		return fmt.Errorf("DB operation timeout must be > 0")
	}
	if cfg.Interview.BudgetSeconds < cfg.Interview.AnswerTimeLimitSeconds {
		return fmt.Errorf("INTERVIEW_BUDGET_SECONDS must be >= ANSWER_TIME_LIMIT_SECONDS")
	}
	if err := validateAsyncRuntime("ASYNC_ANSWER", cfg.Interview.AsyncRuntime); err != nil {
		return err
	}
	if err := validateAsyncRuntime("ASYNC_REPORT", cfg.Report.AsyncRuntime); err != nil {
		return err
	}
	if err := validateAreaConfigs(cfg.Interview.AreaConfigs, cfg.AI.InterviewMidpointAreaIndex); err != nil {
		return err
	}
	if cfg.AI.InterviewClosingSeconds > cfg.AI.InterviewLastQuestionSeconds {
		return fmt.Errorf("AI_INTERVIEW_CLOSING_SECONDS must be <= AI_INTERVIEW_LAST_QUESTION_SECONDS")
	}

	switch cfg.AI.Provider {
	case "claude", "ollama", "vertex":
	default:
		return fmt.Errorf("invalid AI_PROVIDER %q (expected \"claude\", \"ollama\", or \"vertex\")", cfg.AI.Provider)
	}

	if cfg.AI.Provider == "claude" && cfg.AI.APIKey == "" && cfg.AI.MockAPIURL == "" {
		return fmt.Errorf("AI_API_KEY is required (or set MOCK_API_URL for dev)")
	}
	if strings.TrimSpace(cfg.Payment.StripeSecretKey) == "" {
		return fmt.Errorf("STRIPE_SECRET_KEY is required")
	}
	if strings.TrimSpace(cfg.Payment.StripeWebhookSecret) == "" {
		return fmt.Errorf("STRIPE_WEBHOOK_SECRET is required")
	}
	if cfg.AI.Provider == "ollama" {
		if cfg.AI.MockAPIURL != "" {
			slog.Warn("MOCK_API_URL is ignored when AI_PROVIDER=ollama")
		}
		if cfg.AI.UnstructuredInterviewOutputFormatPrompt == "" {
			slog.Warn("UNSTRUCTURED_INTERVIEW_OUTPUT_FORMAT_PROMPT is empty; Ollama interview JSON reliability may be reduced")
		}
		if cfg.AI.UnstructuredReportOutputFormatPrompt == "" {
			slog.Warn("UNSTRUCTURED_REPORT_OUTPUT_FORMAT_PROMPT is empty; Ollama report JSON reliability may be reduced")
		}
	}
	if cfg.AI.Provider == "vertex" {
		cfg.AI.VertexAuthMode = strings.TrimSpace(cfg.AI.VertexAuthMode)
		if cfg.AI.VertexAuthMode != "api_key" && cfg.AI.VertexAuthMode != "adc" {
			return fmt.Errorf("invalid VERTEX_AI_AUTH_MODE %q (expected \"api_key\" or \"adc\")", cfg.AI.VertexAuthMode)
		}
		if strings.TrimSpace(cfg.AI.MockAPIURL) != "" {
			return fmt.Errorf("MOCK_API_URL is not supported when AI_PROVIDER=vertex")
		}
		if strings.TrimSpace(cfg.AI.VertexProjectID) == "" {
			return fmt.Errorf("VERTEX_AI_PROJECT_ID is required when AI_PROVIDER=vertex")
		}
		if cfg.AI.VertexAuthMode == "api_key" && strings.TrimSpace(cfg.AI.VertexAPIKey) == "" {
			return fmt.Errorf("VERTEX_AI_API_KEY is required when AI_PROVIDER=vertex and VERTEX_AI_AUTH_MODE=api_key")
		}
	}

	return nil
}

func validateAsyncRuntime(prefix string, cfg AsyncRuntimeConfig) error {
	if cfg.StaleAfter < cfg.JobTimeout {
		return fmt.Errorf("%s_STALE_AFTER_SECONDS must be >= %s_JOB_TIMEOUT_SECONDS", prefix, prefix)
	}
	return nil
}

func validateAreaConfigs(areas []AreaConfig, midpoint int) error {
	if len(areas) == 0 {
		return fmt.Errorf("AI_AREA_CONFIG must contain at least one area")
	}
	if midpoint < 0 || midpoint >= len(areas) {
		return fmt.Errorf("AI_INTERVIEW_MIDPOINT_AREA_INDEX must be between 0 and %d", len(areas)-1)
	}

	seenIDs := make(map[int]struct{}, len(areas))
	seenSlugs := make(map[string]struct{}, len(areas))
	for i, area := range areas {
		if area.ID <= 0 {
			return fmt.Errorf("AI_AREA_CONFIG[%d].id must be > 0", i)
		}
		slug := strings.TrimSpace(area.Slug)
		if slug == "" {
			return fmt.Errorf("AI_AREA_CONFIG[%d].slug is required", i)
		}
		if _, exists := seenIDs[area.ID]; exists {
			return fmt.Errorf("AI_AREA_CONFIG contains duplicate id %d", area.ID)
		}
		if _, exists := seenSlugs[strings.ToLower(slug)]; exists {
			return fmt.Errorf("AI_AREA_CONFIG contains duplicate slug %q", slug)
		}
		seenIDs[area.ID] = struct{}{}
		seenSlugs[strings.ToLower(slug)] = struct{}{}
	}
	return nil
}

func parseAreaConfigs(areaJSON string) ([]AreaConfig, error) {
	trimmed := strings.TrimSpace(areaJSON)
	if trimmed == "" {
		return nil, fmt.Errorf("AI_AREA_CONFIG must contain at least one area")
	}

	var areas []AreaConfig
	if err := json.Unmarshal([]byte(trimmed), &areas); err != nil {
		return nil, fmt.Errorf("invalid AI_AREA_CONFIG JSON: %w", err)
	}
	return areas, nil
}

func firstNonEmpty(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
