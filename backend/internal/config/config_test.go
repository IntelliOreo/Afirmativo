package config

import (
	"strings"
	"testing"
)

func setBaseValidEnv(t *testing.T) {
	t.Helper()

	env := map[string]string{
		"DATABASE_URL":                                "postgres://user:pass@localhost:5432/app",
		"JWT_SECRET":                                  "test-session-auth-secret-32-bytes-minimum",
		"AI_PROVIDER":                                 "ollama",
		"SESSION_EXPIRY_HOURS":                        "24",
		"AI_MAX_TOKENS":                               "1024",
		"AI_INTERVIEW_LAST_QUESTION_SECONDS":          "30",
		"AI_INTERVIEW_CLOSING_SECONDS":                "15",
		"AI_INTERVIEW_MIDPOINT_AREA_INDEX":            "3",
		"AI_INTERVIEW_PROMPT_CACHING_ENABLED":         "true",
		"AI_TIMEOUT_SECONDS":                          "30",
		"AI_REPORT_MAX_TOKENS":                        "2048",
		"HTTP_READ_TIMEOUT_SECONDS":                   "10",
		"HTTP_WRITE_TIMEOUT_SECONDS":                  "30",
		"HTTP_IDLE_TIMEOUT_SECONDS":                   "60",
		"SESSION_AUTH_MAX_TTL_MINUTES":                "60",
		"OLLAMA_TEMPERATURE":                          "0.3",
		"VOICE_AI_TOKEN_TIMEOUT_SECONDS":              "30",
		"ADMIN_CLEANUP_ENABLED":                       "false",
		"ALLOW_SENSITIVE_DEBUG_LOGS":                  "false",
		"ASYNC_ANSWER_WORKERS":                        "4",
		"ASYNC_ANSWER_QUEUE_SIZE":                     "256",
		"ASYNC_ANSWER_RECOVERY_BATCH":                 "100",
		"ASYNC_ANSWER_RECOVERY_EVERY_SECONDS":         "10",
		"ASYNC_ANSWER_STALE_AFTER_SECONDS":            "180",
		"ASYNC_ANSWER_JOB_TIMEOUT_SECONDS":            "180",
		"VERIFY_IP_RATE_LIMIT_PER_MINUTE":             "60",
		"VERIFY_IP_RATE_LIMIT_BURST":                  "10",
		"VERIFY_FAIL_MAX_ATTEMPTS":                    "5",
		"VERIFY_FAIL_WINDOW_SECONDS":                  "600",
		"VERIFY_FAIL_LOCKOUT_SECONDS":                 "900",
		"VOICE_TOKEN_IP_RATE_LIMIT_PER_MINUTE":        "30",
		"VOICE_TOKEN_IP_RATE_LIMIT_BURST":             "6",
		"VOICE_TOKEN_SESSION_RATE_LIMIT_PER_MINUTE":   "6",
		"VOICE_TOKEN_SESSION_RATE_LIMIT_BURST":        "2",
		"AI_AREA_CONFIG":                              "",
		"AI_API_KEY":                                  "",
		"MOCK_API_URL":                                "",
		"AI_INTERVIEW_SYSTEM_PROMPT":                  "",
		"AI_INTERVIEW_PROMPT_LAST_QUESTION":           "",
		"AI_INTERVIEW_PROMPT_CLOSING":                 "",
		"AI_INTERVIEW_PROMPT_OPENING_TURN":            "",
		"UNSTRUCTURED_INTERVIEW_OUTPUT_FORMAT_PROMPT": "",
		"UNSTRUCTURED_REPORT_OUTPUT_FORMAT_PROMPT":    "",
		"VERTEX_AI_AUTH_MODE":                         "api_key",
		"VERTEX_AI_API_KEY":                           "",
		"VERTEX_AI_PROJECT_ID":                        "",
		"VERTEX_AI_LOCATION":                          "global",
		"VERTEX_AI_EXPLICIT_CACHE_ENABLED":            "true",
		"VERTEX_AI_CONTEXT_CACHE_TTL_SECONDS":         "300",
	}

	for key, value := range env {
		t.Setenv(key, value)
	}
}

func TestLoad_ParsesNewAsyncAndRateLimitKnobs(t *testing.T) {
	setBaseValidEnv(t)

	t.Setenv("ASYNC_ANSWER_WORKERS", "7")
	t.Setenv("ASYNC_ANSWER_QUEUE_SIZE", "321")
	t.Setenv("ASYNC_ANSWER_RECOVERY_BATCH", "22")
	t.Setenv("ASYNC_ANSWER_RECOVERY_EVERY_SECONDS", "11")
	t.Setenv("ASYNC_ANSWER_STALE_AFTER_SECONDS", "240")
	t.Setenv("ASYNC_ANSWER_JOB_TIMEOUT_SECONDS", "200")
	t.Setenv("VERIFY_IP_RATE_LIMIT_PER_MINUTE", "120")
	t.Setenv("VERIFY_IP_RATE_LIMIT_BURST", "20")
	t.Setenv("VERIFY_FAIL_MAX_ATTEMPTS", "6")
	t.Setenv("VERIFY_FAIL_WINDOW_SECONDS", "700")
	t.Setenv("VERIFY_FAIL_LOCKOUT_SECONDS", "950")
	t.Setenv("VOICE_TOKEN_IP_RATE_LIMIT_PER_MINUTE", "40")
	t.Setenv("VOICE_TOKEN_IP_RATE_LIMIT_BURST", "7")
	t.Setenv("VOICE_TOKEN_SESSION_RATE_LIMIT_PER_MINUTE", "8")
	t.Setenv("VOICE_TOKEN_SESSION_RATE_LIMIT_BURST", "3")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.AsyncAnswerWorkers != 7 ||
		cfg.AsyncAnswerQueueSize != 321 ||
		cfg.AsyncAnswerRecoveryBatch != 22 ||
		cfg.AsyncAnswerRecoveryEverySeconds != 11 ||
		cfg.AsyncAnswerStaleAfterSeconds != 240 ||
		cfg.AsyncAnswerJobTimeoutSeconds != 200 {
		t.Fatalf("unexpected async runtime config: %#v", cfg)
	}

	if cfg.VerifyIPRatePerMinute != 120 ||
		cfg.VerifyIPBurst != 20 ||
		cfg.VerifyFailMaxAttempts != 6 ||
		cfg.VerifyFailWindowSeconds != 700 ||
		cfg.VerifyFailLockoutSeconds != 950 {
		t.Fatalf("unexpected verify limiter config: %#v", cfg)
	}

	if cfg.VoiceIPRatePerMinute != 40 ||
		cfg.VoiceIPBurst != 7 ||
		cfg.VoiceSessionRatePerMinute != 8 ||
		cfg.VoiceSessionBurst != 3 {
		t.Fatalf("unexpected voice limiter config: %#v", cfg)
	}
}

func TestLoad_ParsesInterviewPromptCachingFlag(t *testing.T) {
	setBaseValidEnv(t)
	t.Setenv("AI_INTERVIEW_PROMPT_CACHING_ENABLED", "false")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.AIInterviewPromptCachingEnabled {
		t.Fatalf("AIInterviewPromptCachingEnabled = %v, want false", cfg.AIInterviewPromptCachingEnabled)
	}
}

func TestLoad_AcceptsVertexProviderWithAPIKeyAuth(t *testing.T) {
	setBaseValidEnv(t)
	t.Setenv("AI_PROVIDER", "vertex")
	t.Setenv("VERTEX_AI_API_KEY", "vertex-test-key")
	t.Setenv("VERTEX_AI_PROJECT_ID", "afirmativo-dev")
	t.Setenv("VERTEX_AI_CONTEXT_CACHE_TTL_SECONDS", "300")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.AIProvider != "vertex" {
		t.Fatalf("AIProvider = %q, want vertex", cfg.AIProvider)
	}
	if cfg.VertexAIAuthMode != "api_key" {
		t.Fatalf("VertexAIAuthMode = %q, want api_key", cfg.VertexAIAuthMode)
	}
	if cfg.VertexAIContextCacheTTLSeconds != 300 {
		t.Fatalf("VertexAIContextCacheTTLSeconds = %d, want 300", cfg.VertexAIContextCacheTTLSeconds)
	}
}

func TestLoad_RejectsInvalidVertexAuthMode(t *testing.T) {
	setBaseValidEnv(t)
	t.Setenv("AI_PROVIDER", "vertex")
	t.Setenv("VERTEX_AI_AUTH_MODE", "invalid")
	t.Setenv("VERTEX_AI_PROJECT_ID", "afirmativo-dev")
	t.Setenv("VERTEX_AI_API_KEY", "vertex-test-key")

	_, err := Load()
	if err == nil {
		t.Fatalf("Load() expected error for invalid VERTEX_AI_AUTH_MODE")
	}
	if !strings.Contains(err.Error(), `invalid VERTEX_AI_AUTH_MODE`) {
		t.Fatalf("error = %v, want VERTEX_AI_AUTH_MODE validation message", err)
	}
}

func TestLoad_RejectsInvalidAsyncWorkers(t *testing.T) {
	setBaseValidEnv(t)
	t.Setenv("ASYNC_ANSWER_WORKERS", "0")

	_, err := Load()
	if err == nil {
		t.Fatalf("Load() expected error for ASYNC_ANSWER_WORKERS=0")
	}
	if !strings.Contains(err.Error(), "ASYNC_ANSWER_WORKERS must be > 0") {
		t.Fatalf("error = %v, want ASYNC_ANSWER_WORKERS validation message", err)
	}
}

func TestLoad_RejectsInvalidVoiceSessionBurst(t *testing.T) {
	setBaseValidEnv(t)
	t.Setenv("VOICE_TOKEN_SESSION_RATE_LIMIT_BURST", "-1")

	_, err := Load()
	if err == nil {
		t.Fatalf("Load() expected error for VOICE_TOKEN_SESSION_RATE_LIMIT_BURST=-1")
	}
	if !strings.Contains(err.Error(), "VOICE_TOKEN_SESSION_RATE_LIMIT_BURST must be > 0") {
		t.Fatalf("error = %v, want VOICE_TOKEN_SESSION_RATE_LIMIT_BURST validation message", err)
	}
}
