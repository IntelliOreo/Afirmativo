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

const voiceProviderDeepgram = "deepgram"

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
	sessionSvc := session.NewService(sessionStore, cfg.SessionExpiryHours, cfg.InterviewBudgetSeconds)

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
		Window:      time.Duration(cfg.VerifyFailWindowSeconds) * time.Second,
		Lockout:     time.Duration(cfg.VerifyFailLockoutSeconds) * time.Second,
	}))

	interviewStore := interview.NewPostgresStore(pool)

	reportStore := report.NewPostgresStore(pool)

	aiClient, reportAIClient, err := createAIClients(cfg)
	if err != nil {
		slog.Error("failed to initialize AI clients", "error", err)
		os.Exit(1)
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
		cfg.AnswerTimeLimitSeconds,
		interview.AsyncConfig{
			Workers:       cfg.AsyncAnswerWorkers,
			QueueSize:     cfg.AsyncAnswerQueueSize,
			RecoveryBatch: cfg.AsyncAnswerRecoveryBatch,
			RecoveryEvery: time.Duration(cfg.AsyncAnswerRecoveryEverySeconds) * time.Second,
			StaleAfter:    time.Duration(cfg.AsyncAnswerStaleAfterSeconds) * time.Second,
			JobTimeout:    time.Duration(cfg.AsyncAnswerJobTimeoutSeconds) * time.Second,
		},
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
	reportSvc.SetAsyncConfig(report.AsyncConfig{
		Workers:       cfg.AsyncReportWorkers,
		QueueSize:     cfg.AsyncReportQueueSize,
		RecoveryBatch: cfg.AsyncReportRecoveryBatch,
		RecoveryEvery: time.Duration(cfg.AsyncReportRecoveryEverySeconds) * time.Second,
		StaleAfter:    time.Duration(cfg.AsyncReportStaleAfterSeconds) * time.Second,
		JobTimeout:    time.Duration(cfg.AsyncReportJobTimeoutSeconds) * time.Second,
	})
	reportSvc.StartAsyncRuntime(asyncRuntimeCtx)
	reportHandler := report.NewHandler(reportSvc)

	paymentHandler := payment.NewHandler()

	adminStore := admin.NewPostgresStore(pool)
	adminSvc := admin.NewService(adminStore)
	adminHandler := admin.NewHandler(adminSvc)

	voiceClient, err := voice.NewClient(voice.ClientConfig{
		BaseURL:        cfg.VoiceAIBaseURL,
		APIKey:         cfg.VoiceAIAPIKey,
		Model:          cfg.VoiceAIModel,
		Provider:       voiceProviderDeepgram,
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
		RequestsPerMinute: cfg.VoiceSessionRatePerMinute,
		Burst:             cfg.VoiceSessionBurst,
		KeyFunc:           shared.SessionCodeRateLimitKey,
	})

	// Register routes.
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", shared.HandleHealth(pool))
	mux.HandleFunc("POST /api/coupon/validate", sessionHandler.HandleValidateCoupon)
	mux.HandleFunc("POST /api/session/verify", sessionVerifyIPLimiter.Wrap(sessionHandler.HandleVerifySession))
	mux.HandleFunc("GET /api/session/access", shared.RequireSessionAuth(sessionAuth, sessionHandler.HandleCheckAccess))
	mux.HandleFunc("POST /api/interview/start", shared.RequireSessionAuth(sessionAuth, interviewHandler.HandleStart))
	mux.HandleFunc("POST /api/interview/answer-async", shared.RequireSessionAuth(sessionAuth, interviewHandler.HandleAnswerAsync))
	mux.HandleFunc("GET /api/interview/answer-jobs/{jobId}", shared.RequireSessionAuth(sessionAuth, interviewHandler.HandleAnswerJobStatus))
	mux.HandleFunc(
		"POST /api/voice/token",
		shared.RequireSessionAuth(sessionAuth, voiceTokenIPLimiter.Wrap(voiceTokenSessionLimiter.Wrap(voiceHandler.HandleMintToken))),
	)
	mux.HandleFunc("POST /api/report/{code}/generate", shared.RequireSessionAuth(sessionAuth, reportHandler.HandleGenerateReport))
	mux.HandleFunc("GET /api/report/{code}", shared.RequireSessionAuth(sessionAuth, reportHandler.HandleGetReport))
	mux.HandleFunc("GET /api/report/{code}/pdf", shared.RequireSessionAuth(sessionAuth, reportHandler.HandleGetReportPDF))
	mux.HandleFunc("POST /api/payment/checkout", paymentHandler.HandleCheckout)
	mux.HandleFunc("POST /api/payment/webhook", paymentHandler.HandleWebhook)

	if cfg.AdminCleanupEnabled {
		mux.HandleFunc("POST /api/admin/cleanup-db", adminHandler.HandleCleanupDB)
		slog.Warn("admin cleanup endpoint enabled")
	} else {
		slog.Info("admin cleanup endpoint disabled")
	}

	// Apply middleware.
	handler := shared.Chain(mux,
		shared.CORS(cfg.FrontendURL),
		shared.RequestID,
		shared.SecurityHeaders,
		shared.Logger,
	)

	// Start server with graceful shutdown.
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      handler,
		ReadTimeout:  time.Duration(cfg.HTTPReadTimeoutSeconds) * time.Second,
		WriteTimeout: time.Duration(cfg.HTTPWriteTimeoutSeconds) * time.Second,
		IdleTimeout:  time.Duration(cfg.HTTPIdleTimeoutSeconds) * time.Second,
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
