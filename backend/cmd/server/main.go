// Package main is the composition root for the Afirmativo backend.
// It loads config, connects to the database, wires all dependencies,
// registers HTTP routes, and starts the server with graceful shutdown.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/afirmativo/backend/internal/admin"
	"github.com/afirmativo/backend/internal/config"
	"github.com/afirmativo/backend/internal/interview"
	"github.com/afirmativo/backend/internal/payment"
	"github.com/afirmativo/backend/internal/report"
	"github.com/afirmativo/backend/internal/session"
	"github.com/afirmativo/backend/internal/shared"
	"github.com/afirmativo/backend/internal/voice"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func main() {
	// Load .env in local dev; ignore error in containers where env is injected.
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Set log level from config.
	var logLevel slog.Level
	switch strings.ToLower(cfg.LogLevel) {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	})))

	cfg.LogLoaded()

	// Connect to Postgres.
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		slog.Error("failed to ping database", "error", err)
		os.Exit(1)
	}
	slog.Info("database connected")

	// Wire dependencies.
	sessionStore := session.NewPostgresStore(pool)
	sessionSvc := session.NewService(sessionStore, cfg.SessionExpiryHours)

	useSecureAuthCookie := strings.HasPrefix(strings.ToLower(cfg.FrontendURL), "https://")
	sessionAuth, err := shared.NewSessionAuthManager(shared.SessionAuthConfig{
		Secret:       cfg.JWTSecret,
		CookieName:   cfg.SessionAuthCookieName,
		Issuer:       cfg.SessionAuthIssuer,
		Audience:     cfg.SessionAuthAudience,
		CookieSecure: useSecureAuthCookie,
	})
	if err != nil {
		slog.Error("failed to initialize session auth manager", "error", err)
		os.Exit(1)
	}
	slog.Debug("session auth configured",
		"cookie_name", sessionAuth.CookieName(),
		"cookie_secure", useSecureAuthCookie,
		"max_ttl_minutes", cfg.SessionAuthMaxTTLMinutes,
	)
	sessionHandler := session.NewHandler(sessionSvc, sessionAuth, time.Duration(cfg.SessionAuthMaxTTLMinutes)*time.Minute)
	sessionHandler.SetVerifyAttemptLimiter(shared.NewFailedAttemptLockoutLimiter(shared.FailedAttemptLockoutConfig{
		Name:        "session_verify_fail_lockout",
		MaxFailures: cfg.VerifyFailMaxAttempts,
		Window:      time.Duration(cfg.VerifyFailWindowS) * time.Second,
		Lockout:     time.Duration(cfg.VerifyFailLockoutS) * time.Second,
	}))

	interviewStore := interview.NewPostgresStore(pool)

	reportStore := report.NewPostgresStore(pool)

	var aiClient interview.AIClient
	var reportAIClient report.AIClient

	switch cfg.AIProvider {
	case "ollama":
		aiClient = interview.NewOllamaAIClient(interview.OllamaAIClientConfig{
			BaseURL:                 cfg.OllamaBaseURL,
			Model:                   cfg.AIModel,
			MaxTokens:               cfg.AIMaxTokens,
			Temperature:             cfg.OllamaTemperature,
			AllowSensitiveDebugLogs: cfg.AllowSensitiveDebugLogs,
			SystemPrompt:            cfg.AISystemPrompt,
			OutputFormatPrompt:      cfg.UnstructuredInterviewOutputFormatPrompt,
			PromptLastQ:             cfg.AIPromptLastQ,
			PromptClosing:           cfg.AIPromptClosing,
			LastQSeconds:            cfg.AILastQSeconds,
			ClosingSeconds:          cfg.AIClosingSeconds,
			MidpointAreaIndex:       cfg.AIMidpointAreaIdx,
			TimeoutSeconds:          cfg.AITimeoutSeconds,
			AreaConfigs:             cfg.AreaConfigs,
		})
		reportAIClient = report.NewOllamaReportAIClient(report.OllamaReportAIClientConfig{
			BaseURL:                 cfg.OllamaBaseURL,
			Model:                   cfg.AIModel,
			MaxTokens:               cfg.AIReportMaxTokens,
			Temperature:             cfg.OllamaTemperature,
			TimeoutSeconds:          cfg.AITimeoutSeconds,
			ReportPrompt:            cfg.AIReportPrompt,
			OutputFormatPrompt:      cfg.UnstructuredReportOutputFormatPrompt,
			AllowSensitiveDebugLogs: cfg.AllowSensitiveDebugLogs,
		})
	default:
		// Claude branch: use mock server URL if set, otherwise real Anthropic API.
		aiBaseURL := "https://api.anthropic.com"
		if cfg.MockAPIURL != "" {
			aiBaseURL = cfg.MockAPIURL
		}
		aiClient = interview.NewHTTPAIClient(interview.AIClientConfig{
			BaseURL:                 aiBaseURL,
			APIKey:                  cfg.AIAPIKey,
			Model:                   cfg.AIModel,
			MaxTokens:               cfg.AIMaxTokens,
			AllowSensitiveDebugLogs: cfg.AllowSensitiveDebugLogs,
			SystemPrompt:            cfg.AISystemPrompt,
			PromptLastQ:             cfg.AIPromptLastQ,
			PromptClosing:           cfg.AIPromptClosing,
			LastQSeconds:            cfg.AILastQSeconds,
			ClosingSeconds:          cfg.AIClosingSeconds,
			MidpointAreaIndex:       cfg.AIMidpointAreaIdx,
			TimeoutSeconds:          cfg.AITimeoutSeconds,
			AreaConfigs:             cfg.AreaConfigs,
		})
		reportAIClient = report.NewHTTPReportAIClient(report.ReportAIClientConfig{
			BaseURL:                 aiBaseURL,
			APIKey:                  cfg.AIAPIKey,
			Model:                   cfg.AIModel,
			MaxTokens:               cfg.AIReportMaxTokens,
			TimeoutSeconds:          cfg.AITimeoutSeconds,
			ReportPrompt:            cfg.AIReportPrompt,
			AllowSensitiveDebugLogs: cfg.AllowSensitiveDebugLogs,
		})
	}

	interviewSvc := interview.NewService(
		sessionStore,
		sessionStore,
		sessionStore,
		interviewStore,
		aiClient,
		cfg.AreaConfigs,
		cfg.InterviewOpeningDisclaimerEn,
		cfg.InterviewOpeningDisclaimerEs,
		cfg.InterviewReadinessQuestionEn,
		cfg.InterviewReadinessQuestionEs,
	)
	interviewSvc.ConfigureAsyncAnswerRuntime(
		cfg.AsyncAnswerWorkers,
		cfg.AsyncAnswerQueueSize,
		cfg.AsyncAnswerRecoveryBatch,
		time.Duration(cfg.AsyncAnswerRecoveryEvery)*time.Second,
		time.Duration(cfg.AsyncAnswerStaleAfterS)*time.Second,
		time.Duration(cfg.AsyncAnswerJobTimeoutS)*time.Second,
	)
	asyncRuntimeCtx, asyncRuntimeCancel := context.WithCancel(context.Background())
	defer asyncRuntimeCancel()
	interviewSvc.StartAsyncAnswerRuntime(asyncRuntimeCtx)

	interviewHandler := interview.NewHandler(interviewSvc)
	interviewHandler.SetAllowSensitiveDebugLogs(cfg.AllowSensitiveDebugLogs)

	// Report dependencies.
	reportSvc := report.NewService(
		reportStore,
		&interviewDataAdapter{store: interviewStore},
		&sessionDataAdapter{store: sessionStore},
		reportAIClient,
		cfg.AreaConfigs,
	)
	reportHandler := report.NewHandler(reportSvc)

	paymentHandler := payment.NewHandler()

	adminStore := admin.NewPostgresStore(pool)
	adminSvc := admin.NewService(adminStore)
	adminHandler := admin.NewHandler(adminSvc)

	voiceClient, err := voice.NewClient(voice.ClientConfig{
		BaseURL:        cfg.VoiceAIBaseURL,
		APIKey:         cfg.VoiceAIAPIKey,
		Model:          cfg.VoiceAIModel,
		Provider:       cfg.AIProvider,
		TimeoutSeconds: cfg.AITimeoutSeconds,
	})
	if err != nil {
		slog.Error("failed to initialize voice client", "error", err)
		os.Exit(1)
	}
	voiceHandler := voice.NewHandler(voiceClient, cfg.VoiceAITokenTimeoutSeconds)
	sessionVerifyIPLimiter := shared.NewTokenBucketRateLimiter(shared.TokenBucketRateLimiterConfig{
		Name:              "session_verify_ip",
		RequestsPerMinute: cfg.VerifyIPRatePerMinute,
		Burst:             cfg.VerifyIPBurst,
		KeyFunc:           shared.ClientIPRateLimitKey,
	})
	voiceTokenIPLimiter := shared.NewTokenBucketRateLimiter(shared.TokenBucketRateLimiterConfig{
		Name:              "voice_token_ip",
		RequestsPerMinute: cfg.VoiceIPRatePerMinute,
		Burst:             cfg.VoiceIPBurst,
		KeyFunc:           shared.ClientIPRateLimitKey,
	})
	voiceTokenSessionLimiter := shared.NewTokenBucketRateLimiter(shared.TokenBucketRateLimiterConfig{
		Name:              "voice_token_session",
		RequestsPerMinute: cfg.VoiceSessionRatePerMin,
		Burst:             cfg.VoiceSessionBurst,
		KeyFunc:           shared.SessionCodeRateLimitKey,
	})

	// Register routes.
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", shared.HandleHealth(pool))
	mux.HandleFunc("POST /api/coupon/validate", sessionHandler.HandleValidateCoupon)
	mux.HandleFunc("POST /api/session/verify", sessionVerifyIPLimiter.Wrap(sessionHandler.HandleVerifySession))
	mux.HandleFunc("POST /api/interview/start", shared.RequireSessionAuth(sessionAuth, interviewHandler.HandleStart))
	mux.HandleFunc("POST /api/interview/answer-async", shared.RequireSessionAuth(sessionAuth, interviewHandler.HandleAnswerAsync))
	mux.HandleFunc("GET /api/interview/answer-jobs/{jobId}", shared.RequireSessionAuth(sessionAuth, interviewHandler.HandleAnswerJobStatus))
	mux.HandleFunc(
		"POST /api/deepgram/token",
		shared.RequireSessionAuth(sessionAuth, voiceTokenIPLimiter.Wrap(voiceTokenSessionLimiter.Wrap(voiceHandler.HandleMintToken))),
	)
	mux.HandleFunc("GET /api/report/{code}", shared.RequireSessionAuth(sessionAuth, reportHandler.HandleGetReport))
	mux.HandleFunc("GET /api/report/{code}/pdf", shared.RequireSessionAuth(sessionAuth, reportHandler.HandleGetReportPDF))
	mux.HandleFunc("POST /api/payment/checkout", paymentHandler.HandleCheckout)
	mux.HandleFunc("POST /api/payment/webhook", paymentHandler.HandleWebhook)

	if cfg.AdminCleanupEnabled {
		mux.HandleFunc("POST /api/admin/clean-up-db", adminHandler.HandleCleanUpDB)
		slog.Warn("admin cleanup endpoint enabled")
	} else {
		slog.Info("admin cleanup endpoint disabled")
	}

	// Apply middleware.
	handler := shared.Chain(mux,
		shared.CORS(cfg.FrontendURL),
		shared.SecurityHeaders,
		shared.Logger,
	)

	// Start server with graceful shutdown.
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      handler,
		ReadTimeout:  time.Duration(cfg.HTTPReadTimeoutS) * time.Second,
		WriteTimeout: time.Duration(cfg.HTTPWriteTimeoutS) * time.Second,
		IdleTimeout:  time.Duration(cfg.HTTPIdleTimeoutS) * time.Second,
	}

	go func() {
		slog.Info("server starting", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down server")
	asyncRuntimeCancel()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server forced shutdown", "error", err)
		os.Exit(1)
	}
	slog.Info("server stopped")
}
