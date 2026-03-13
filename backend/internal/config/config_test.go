package config

import (
	"strings"
	"testing"
	"time"
)

const stubAreaConfigJSON = `[
	{"id":1,"slug":"area_1","label":"Area 1","description":"Stub description 1","sufficiency_requirements":"Stub requirement 1","fallback_question":"Stub question 1?"},
	{"id":2,"slug":"area_2","label":"Area 2","description":"Stub description 2","sufficiency_requirements":"Stub requirement 2","fallback_question":"Stub question 2?"},
	{"id":3,"slug":"area_3","label":"Area 3","description":"Stub description 3","sufficiency_requirements":"Stub requirement 3","fallback_question":"Stub question 3?"},
	{"id":4,"slug":"area_4","label":"Area 4","description":"Stub description 4","sufficiency_requirements":"Stub requirement 4","fallback_question":"Stub question 4?"}
]`

func setBaseValidEnv(t *testing.T) {
	t.Helper()

	env := map[string]string{
		"DATABASE_URL":                                "postgres://user:pass@localhost:5432/app",
		"JWT_SECRET":                                  "test-session-auth-secret-32-bytes-minimum",
		"AI_PROVIDER":                                 "ollama",
		"SESSION_EXPIRY_HOURS":                        "24",
		"INTERVIEW_BUDGET_SECONDS":                    "2400",
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
		"ASYNC_REPORT_WORKERS":                        "2",
		"ASYNC_REPORT_QUEUE_SIZE":                     "64",
		"ASYNC_REPORT_RECOVERY_BATCH":                 "50",
		"ASYNC_REPORT_RECOVERY_EVERY_SECONDS":         "10",
		"ASYNC_REPORT_STALE_AFTER_SECONDS":            "180",
		"ASYNC_REPORT_JOB_TIMEOUT_SECONDS":            "180",
		"VERIFY_IP_RATE_LIMIT_PER_MINUTE":             "60",
		"VERIFY_IP_RATE_LIMIT_BURST":                  "10",
		"VERIFY_FAIL_MAX_ATTEMPTS":                    "5",
		"VERIFY_FAIL_WINDOW_SECONDS":                  "600",
		"VERIFY_FAIL_LOCKOUT_SECONDS":                 "900",
		"VOICE_TOKEN_IP_RATE_LIMIT_PER_MINUTE":        "30",
		"VOICE_TOKEN_IP_RATE_LIMIT_BURST":             "6",
		"VOICE_TOKEN_SESSION_RATE_LIMIT_PER_MINUTE":   "6",
		"VOICE_TOKEN_SESSION_RATE_LIMIT_BURST":        "2",
		"AI_AREA_CONFIG":                              stubAreaConfigJSON,
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
	t.Setenv("ASYNC_REPORT_WORKERS", "3")
	t.Setenv("ASYNC_REPORT_QUEUE_SIZE", "111")
	t.Setenv("ASYNC_REPORT_RECOVERY_BATCH", "19")
	t.Setenv("ASYNC_REPORT_RECOVERY_EVERY_SECONDS", "14")
	t.Setenv("ASYNC_REPORT_STALE_AFTER_SECONDS", "260")
	t.Setenv("ASYNC_REPORT_JOB_TIMEOUT_SECONDS", "210")
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

	if cfg.Interview.AsyncRuntime.Workers != 7 ||
		cfg.Interview.AsyncRuntime.QueueSize != 321 ||
		cfg.Interview.AsyncRuntime.RecoveryBatch != 22 ||
		cfg.Interview.AsyncRuntime.RecoveryEvery != 11*time.Second ||
		cfg.Interview.AsyncRuntime.StaleAfter != 240*time.Second ||
		cfg.Interview.AsyncRuntime.JobTimeout != 200*time.Second {
		t.Fatalf("unexpected async runtime config: %#v", cfg)
	}
	if cfg.Report.AsyncRuntime.Workers != 3 ||
		cfg.Report.AsyncRuntime.QueueSize != 111 ||
		cfg.Report.AsyncRuntime.RecoveryBatch != 19 ||
		cfg.Report.AsyncRuntime.RecoveryEvery != 14*time.Second ||
		cfg.Report.AsyncRuntime.StaleAfter != 260*time.Second ||
		cfg.Report.AsyncRuntime.JobTimeout != 210*time.Second {
		t.Fatalf("unexpected async report config: %#v", cfg)
	}

	if cfg.RateLimit.Verify.IPRatePerMinute != 120 ||
		cfg.RateLimit.Verify.IPBurst != 20 ||
		cfg.RateLimit.Verify.FailMaxAttempts != 6 ||
		cfg.RateLimit.Verify.FailWindow != 700*time.Second ||
		cfg.RateLimit.Verify.FailLockout != 950*time.Second {
		t.Fatalf("unexpected verify limiter config: %#v", cfg)
	}

	if cfg.RateLimit.Voice.IPRatePerMinute != 40 ||
		cfg.RateLimit.Voice.IPBurst != 7 ||
		cfg.RateLimit.Voice.SessionRatePerMinute != 8 ||
		cfg.RateLimit.Voice.SessionBurst != 3 {
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
	if cfg.AI.InterviewPromptCachingEnabled {
		t.Fatalf("AI.InterviewPromptCachingEnabled = %v, want false", cfg.AI.InterviewPromptCachingEnabled)
	}
}

func TestLoad_ParsesInterviewBudgetSeconds(t *testing.T) {
	setBaseValidEnv(t)
	t.Setenv("INTERVIEW_BUDGET_SECONDS", "1800")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Interview.BudgetSeconds != 1800 {
		t.Fatalf("Interview.BudgetSeconds = %d, want 1800", cfg.Interview.BudgetSeconds)
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
	if cfg.AI.Provider != "vertex" {
		t.Fatalf("AI.Provider = %q, want vertex", cfg.AI.Provider)
	}
	if cfg.AI.VertexAuthMode != "api_key" {
		t.Fatalf("AI.VertexAuthMode = %q, want api_key", cfg.AI.VertexAuthMode)
	}
	if cfg.AI.VertexContextCacheTTL != 300*time.Second {
		t.Fatalf("AI.VertexContextCacheTTL = %v, want 300s", cfg.AI.VertexContextCacheTTL)
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

func TestLoad_RejectsVertexProviderWithMockAPIURL(t *testing.T) {
	setBaseValidEnv(t)
	t.Setenv("AI_PROVIDER", "vertex")
	t.Setenv("VERTEX_AI_AUTH_MODE", "adc")
	t.Setenv("VERTEX_AI_PROJECT_ID", "afirmativo-dev")
	t.Setenv("MOCK_API_URL", "http://localhost:9999")

	_, err := Load()
	if err == nil {
		t.Fatalf("Load() expected error for vertex mock mode")
	}
	if !strings.Contains(err.Error(), "MOCK_API_URL is not supported when AI_PROVIDER=vertex") {
		t.Fatalf("error = %v, want vertex mock validation message", err)
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

func TestLoad_RejectsInvalidInterviewBudgetSeconds(t *testing.T) {
	setBaseValidEnv(t)
	t.Setenv("INTERVIEW_BUDGET_SECONDS", "0")

	_, err := Load()
	if err == nil {
		t.Fatalf("Load() expected error for INTERVIEW_BUDGET_SECONDS=0")
	}
	if !strings.Contains(err.Error(), "INTERVIEW_BUDGET_SECONDS must be > 0") {
		t.Fatalf("error = %v, want INTERVIEW_BUDGET_SECONDS validation message", err)
	}
}

func TestLoad_RejectsInterviewBudgetSmallerThanAnswerTimeLimit(t *testing.T) {
	setBaseValidEnv(t)
	t.Setenv("INTERVIEW_BUDGET_SECONDS", "299")
	t.Setenv("ANSWER_TIME_LIMIT_SECONDS", "300")

	_, err := Load()
	if err == nil {
		t.Fatalf("Load() expected error for budget smaller than answer time limit")
	}
	if !strings.Contains(err.Error(), "INTERVIEW_BUDGET_SECONDS must be >= ANSWER_TIME_LIMIT_SECONDS") {
		t.Fatalf("error = %v, want interview budget cross-field validation message", err)
	}
}

func TestLoad_RejectsInterviewAsyncStaleAfterShorterThanJobTimeout(t *testing.T) {
	setBaseValidEnv(t)
	t.Setenv("ASYNC_ANSWER_STALE_AFTER_SECONDS", "179")
	t.Setenv("ASYNC_ANSWER_JOB_TIMEOUT_SECONDS", "180")

	_, err := Load()
	if err == nil {
		t.Fatalf("Load() expected error for stale-after shorter than job timeout")
	}
	if !strings.Contains(err.Error(), "ASYNC_ANSWER_STALE_AFTER_SECONDS must be >= ASYNC_ANSWER_JOB_TIMEOUT_SECONDS") {
		t.Fatalf("error = %v, want async timing validation message", err)
	}
}

func TestLoad_RejectsDuplicateAreaSlugs(t *testing.T) {
	setBaseValidEnv(t)
	t.Setenv("AI_AREA_CONFIG", `[{"id":1,"slug":"area_1","label":"A"},{"id":2,"slug":"area_1","label":"B"},{"id":3,"slug":"area_3","label":"C"},{"id":4,"slug":"area_4","label":"D"}]`)

	_, err := Load()
	if err == nil {
		t.Fatalf("Load() expected error for duplicate area slug")
	}
	if !strings.Contains(err.Error(), `AI_AREA_CONFIG contains duplicate slug "area_1"`) {
		t.Fatalf("error = %v, want duplicate slug validation message", err)
	}
}
