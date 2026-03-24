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

	// Set log level and format from config.
	var logLevel slog.Level
	switch strings.ToLower(cfg.Server.LogLevel) {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}
	logOpts := &slog.HandlerOptions{Level: logLevel}
	var logHandler slog.Handler
	switch strings.ToLower(cfg.Server.LogFormat) {
	case "json":
		logHandler = shared.NewGCPJSONHandler(os.Stdout, logOpts)
	default:
		logHandler = slog.NewTextHandler(os.Stdout, logOpts)
	}
	slog.SetDefault(slog.New(logHandler))
	instanceMeta := shared.CurrentInstanceMetadata()

	cfg.LogLoaded()

	// Initialize OpenTelemetry (noop when disabled).
	ctx := context.Background()
	otelShutdown, err := shared.InitOTel(ctx, shared.OTelConfig{
		Enabled:           cfg.OTel.Enabled,
		GCPProjectID:      cfg.OTel.GCPProjectID,
		ServiceName:       "afirmativo-backend",
		ServiceInstanceID: instanceMeta.ID,
	})
	if err != nil {
		slog.Error("failed to initialize otel", "error", err)
		os.Exit(1)
	}
	defer otelShutdown(ctx)

	// Connect to Postgres.
	pool, err := pgxpool.New(ctx, cfg.Server.DatabaseURL)
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
	sessionSvc := session.NewService(session.Deps{Store: sessionStore}, session.Settings{
		ExpiryHours:            cfg.Auth.SessionExpiryHours,
		InterviewBudgetSeconds: cfg.Interview.BudgetSeconds,
		DBTimeout:              cfg.DBOperationTimeout,
	})

	useSecureAuthCookie := strings.HasPrefix(strings.ToLower(cfg.Server.FrontendURL), "https://")
	sessionAuth, err := shared.NewSessionAuthManager(shared.SessionAuthConfig{
		Secret:       cfg.Auth.JWTSecret,
		CookieName:   cfg.Auth.SessionAuthCookieName,
		Issuer:       cfg.Auth.SessionAuthIssuer,
		Audience:     cfg.Auth.SessionAuthAudience,
		CookieSecure: useSecureAuthCookie,
	})
	if err != nil {
		slog.Error("failed to initialize session auth manager", "error", err)
		os.Exit(1)
	}
	slog.Debug("session auth configured",
		"cookie_name", sessionAuth.CookieName(),
		"cookie_secure", useSecureAuthCookie,
		"max_ttl_minutes", int(cfg.Auth.SessionAuthMaxTTL/time.Minute),
	)
	sessionHandler := session.NewHandler(sessionSvc, sessionAuth, cfg.Auth.SessionAuthMaxTTL)
	sessionHandler.SetVerifyAttemptLimiter(shared.NewFailedAttemptLockoutLimiter(shared.FailedAttemptLockoutConfig{
		Name:        "session_verify_fail_lockout",
		MaxFailures: cfg.RateLimit.Verify.FailMaxAttempts,
		Window:      cfg.RateLimit.Verify.FailWindow,
		Lockout:     cfg.RateLimit.Verify.FailLockout,
	}))

	interviewStore := interview.NewPostgresStore(pool)

	reportStore := report.NewPostgresStore(pool)

	aiClient, reportAIClient, err := createAIClients(cfg)
	if err != nil {
		slog.Error("failed to initialize AI clients", "error", err)
		os.Exit(1)
	}

	interviewSvc := interview.NewService(interview.Deps{
		SessionStarter:   sessionStore,
		SessionGetter:    sessionStore,
		SessionCompleter: sessionStore,
		Store:            interviewStore,
		AIClient:         aiClient,
	}, interview.Settings{
		AreaConfigs:            cfg.Interview.AreaConfigs,
		OpeningDisclaimer:      cfg.Interview.OpeningDisclaimer,
		ReadinessQuestion:      cfg.Interview.ReadinessQuestion,
		AnswerTimeLimitSeconds: cfg.Interview.AnswerTimeLimitSeconds,
		DBTimeout:              cfg.DBOperationTimeout,
		AsyncRuntime:           cfg.Interview.AsyncRuntime,
	})
	asyncRuntimeCtx, asyncRuntimeCancel := context.WithCancel(context.Background())
	defer asyncRuntimeCancel()
	interviewSvc.StartAsyncAnswerRuntime(asyncRuntimeCtx)

	interviewHandler := interview.NewHandler(interviewSvc)
	interviewHandler.SetAllowSensitiveDebugLogs(cfg.Server.AllowSensitiveDebugLogs)

	// Report dependencies.
	reportSvc := report.NewService(report.Deps{
		Store:      reportStore,
		Interviews: &interviewDataAdapter{store: interviewStore},
		Sessions:   &sessionDataAdapter{store: sessionStore},
		AIClient:   reportAIClient,
	}, report.Settings{
		AreaConfigs:  cfg.Interview.AreaConfigs,
		DBTimeout:    cfg.DBOperationTimeout,
		AsyncRuntime: cfg.Report.AsyncRuntime,
	})
	reportSvc.StartAsyncRuntime(asyncRuntimeCtx)
	reportHandler := report.NewHandler(reportSvc)

	paymentStore := payment.NewPostgresStore(pool)
	stripeClient := payment.NewStripeClient(payment.StripeClientConfig{
		SecretKey:     cfg.Payment.StripeSecretKey,
		WebhookSecret: cfg.Payment.StripeWebhookSecret,
	})
	paymentSvc := payment.NewService(payment.Deps{
		Store:  paymentStore,
		Stripe: stripeClient,
	}, payment.Settings{
		FrontendURL:            cfg.Server.FrontendURL,
		SessionExpiryHours:     cfg.Auth.SessionExpiryHours,
		InterviewBudgetSeconds: cfg.Interview.BudgetSeconds,
		DirectSession: payment.ProductConfig{
			AmountCents: cfg.Payment.DirectSessionAmountCents,
			Currency:    "usd",
			ProductName: "Afirmativo Session Access",
		},
		CouponPack10: payment.ProductConfig{
			AmountCents: cfg.Payment.CouponPack10AmountCents,
			Currency:    "usd",
			ProductName: "Afirmativo 10-Use Coupon Pack",
		},
	})
	paymentHandler := payment.NewHandler(paymentSvc)

	adminStore := admin.NewPostgresStore(pool)
	adminSvc := admin.NewService(adminStore)
	adminHandler := admin.NewHandler(adminSvc)

	voiceClient, err := voice.NewClient(voice.ClientConfig{
		BaseURL:        cfg.Voice.BaseURL,
		APIKey:         cfg.Voice.APIKey,
		Model:          cfg.Voice.Model,
		Provider:       voiceProviderDeepgram,
		TimeoutSeconds: int(cfg.Voice.Timeout / time.Second),
	})
	if err != nil {
		slog.Error("failed to initialize voice client", "error", err)
		os.Exit(1)
	}
	voiceHandler := voice.NewHandler(voiceClient, cfg.Voice.TokenTimeoutSeconds)
	sessionVerifyIPLimiter := shared.NewTokenBucketRateLimiter(shared.TokenBucketRateLimiterConfig{
		Name:              "session_verify_ip",
		RequestsPerMinute: cfg.RateLimit.Verify.IPRatePerMinute,
		Burst:             cfg.RateLimit.Verify.IPBurst,
		KeyFunc:           shared.ClientIPRateLimitKey,
	})
	voiceTokenIPLimiter := shared.NewTokenBucketRateLimiter(shared.TokenBucketRateLimiterConfig{
		Name:              "voice_token_ip",
		RequestsPerMinute: cfg.RateLimit.Voice.IPRatePerMinute,
		Burst:             cfg.RateLimit.Voice.IPBurst,
		KeyFunc:           shared.ClientIPRateLimitKey,
	})
	voiceTokenSessionLimiter := shared.NewTokenBucketRateLimiter(shared.TokenBucketRateLimiterConfig{
		Name:              "voice_token_session",
		RequestsPerMinute: cfg.RateLimit.Voice.SessionRatePerMinute,
		Burst:             cfg.RateLimit.Voice.SessionBurst,
		KeyFunc:           shared.SessionCodeRateLimitKey,
	})

	// Register routes.
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", shared.HandleHealth(pool, Version, instanceMeta, interviewSvc, reportSvc, &poolStatsProvider{pool: pool}))
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
	mux.HandleFunc("GET /api/payment/checkout-sessions/{id}", paymentHandler.HandleCheckoutSessionStatus)

	if cfg.Admin.CleanupEnabled {
		mux.HandleFunc("POST /api/admin/cleanup-db", adminHandler.HandleCleanupDB)
		slog.Warn("admin cleanup endpoint enabled")
	} else {
		slog.Info("admin cleanup endpoint disabled")
	}

	// Apply middleware.
	handler := shared.Chain(mux,
		shared.CORS(cfg.Server.FrontendURL),
		shared.RequestID,
		shared.Trace,
		shared.SecurityHeaders,
		shared.Logger,
		shared.Recovery,
	)

	// Start server with graceful shutdown.
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Server.Port),
		Handler:      handler,
		ReadTimeout:  cfg.Server.HTTPReadTimeout,
		WriteTimeout: cfg.Server.HTTPWriteTimeout,
		IdleTimeout:  cfg.Server.HTTPIdleTimeout,
	}

	go func() {
		slog.Info("server starting",
			"port", cfg.Server.Port,
			"version", Version,
			"instance_id", instanceMeta.ID,
			"service_revision", instanceMeta.Revision,
		)
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

	// 1. Stop accepting new HTTP connections and drain in-flight requests.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server forced shutdown", "error", err)
	}

	// 2. Cancel async runtimes (no new jobs can arrive from HTTP handlers).
	asyncRuntimeCancel()

	// 3. Wait for in-progress workers to finish current jobs.
	drainCtx, drainCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer drainCancel()
	interviewSvc.WaitForDrain(drainCtx)
	reportSvc.WaitForDrain(drainCtx)

	slog.Info("server stopped")
}
