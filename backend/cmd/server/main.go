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

	"github.com/afirmativo/backend/internal/config"
	"github.com/afirmativo/backend/internal/interview"
	"github.com/afirmativo/backend/internal/report"
	"github.com/afirmativo/backend/internal/session"
	"github.com/afirmativo/backend/internal/shared"
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
	sessionHandler := session.NewHandler(sessionSvc)

	interviewStore := interview.NewPostgresStore(pool)

	reportStore := report.NewPostgresStore(pool)

	var aiClient interview.AIClient
	var reportAIClient report.AIClient

	switch cfg.AIProvider {
	case "ollama":
		aiClient = interview.NewOllamaAIClient(interview.OllamaAIClientConfig{
			BaseURL:            cfg.OllamaBaseURL,
			Model:              cfg.AIModel,
			SystemPrompt:       cfg.AISystemPrompt,
			OutputFormatPrompt: cfg.UnstructuredInterviewOutputFormatPrompt,
			PromptLastQ:        cfg.AIPromptLastQ,
			PromptClosing:      cfg.AIPromptClosing,
			LastQSeconds:       cfg.AILastQSeconds,
			ClosingSeconds:     cfg.AIClosingSeconds,
			MidpointAreaIndex:  cfg.AIMidpointAreaIdx,
			TimeoutSeconds:     cfg.AITimeoutSeconds,
			AreaConfigs:        cfg.AreaConfigs,
		})
		reportAIClient = report.NewOllamaReportAIClient(report.OllamaReportAIClientConfig{
			BaseURL:            cfg.OllamaBaseURL,
			Model:              cfg.AIModel,
			TimeoutSeconds:     cfg.AITimeoutSeconds,
			ReportPrompt:       cfg.AIReportPrompt,
			OutputFormatPrompt: cfg.UnstructuredReportOutputFormatPrompt,
		})
	default:
		// Claude branch: use mock server URL if set, otherwise real Anthropic API.
		aiBaseURL := "https://api.anthropic.com"
		if cfg.MockAPIURL != "" {
			aiBaseURL = cfg.MockAPIURL
		}
		aiClient = interview.NewHTTPAIClient(interview.AIClientConfig{
			BaseURL:           aiBaseURL,
			APIKey:            cfg.AIAPIKey,
			Model:             cfg.AIModel,
			MaxTokens:         cfg.AIMaxTokens,
			SystemPrompt:      cfg.AISystemPrompt,
			PromptLastQ:       cfg.AIPromptLastQ,
			PromptClosing:     cfg.AIPromptClosing,
			LastQSeconds:      cfg.AILastQSeconds,
			ClosingSeconds:    cfg.AIClosingSeconds,
			MidpointAreaIndex: cfg.AIMidpointAreaIdx,
			TimeoutSeconds:    cfg.AITimeoutSeconds,
			AreaConfigs:       cfg.AreaConfigs,
		})
		reportAIClient = report.NewHTTPReportAIClient(report.ReportAIClientConfig{
			BaseURL:        aiBaseURL,
			APIKey:         cfg.AIAPIKey,
			Model:          cfg.AIModel,
			MaxTokens:      cfg.AIReportMaxTokens,
			TimeoutSeconds: cfg.AITimeoutSeconds,
			ReportPrompt:   cfg.AIReportPrompt,
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
	interviewHandler := interview.NewHandler(interviewSvc)

	// Report dependencies.
	reportSvc := report.NewService(
		reportStore,
		&interviewDataAdapter{store: interviewStore},
		&sessionDataAdapter{store: sessionStore},
		reportAIClient,
		cfg.AreaConfigs,
	)
	reportHandler := report.NewHandler(reportSvc)

	// Register routes.
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", shared.HandleHealth(pool))
	mux.HandleFunc("POST /api/coupon/validate", sessionHandler.HandleValidateCoupon)
	mux.HandleFunc("POST /api/session/verify", sessionHandler.HandleVerifySession)
	mux.HandleFunc("POST /api/interview/start", interviewHandler.HandleStart)
	mux.HandleFunc("POST /api/interview/answer", interviewHandler.HandleAnswer)
	mux.HandleFunc("GET /api/report/{code}", reportHandler.HandleGetReport)
	mux.HandleFunc("GET /api/report/{code}/pdf", reportHandler.HandleGetReportPDF)

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
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server forced shutdown", "error", err)
		os.Exit(1)
	}
	slog.Info("server stopped")
}

// ── Adapters ────────────────────────────────────────────────────────
// These bridge between the existing domain packages and the report
// package's interfaces, avoiding direct cross-domain imports.

// interviewDataAdapter adapts interview.PostgresStore to report.InterviewDataProvider.
type interviewDataAdapter struct {
	store *interview.PostgresStore
}

func (a *interviewDataAdapter) GetAreasBySession(ctx context.Context, sessionCode string) ([]report.QuestionAreaRow, error) {
	areas, err := a.store.GetAreasBySession(ctx, sessionCode)
	if err != nil {
		return nil, err
	}
	result := make([]report.QuestionAreaRow, len(areas))
	for i, area := range areas {
		result[i] = report.QuestionAreaRow{
			Area:                 area.Area,
			Status:               string(area.Status),
			PreAddressedEvidence: area.PreAddressedEvidence,
		}
	}
	return result, nil
}

func (a *interviewDataAdapter) GetAnswersBySession(ctx context.Context, sessionCode string) ([]report.AnswerRow, error) {
	answers, err := a.store.GetAnswersBySession(ctx, sessionCode)
	if err != nil {
		return nil, err
	}
	result := make([]report.AnswerRow, len(answers))
	for i, ans := range answers {
		result[i] = report.AnswerRow{
			Area:         ans.Area,
			QuestionText: ans.QuestionText,
			TranscriptEs: ans.TranscriptEs,
			AiEvaluation: ans.AiEvaluation,
			Sufficiency:  ans.Sufficiency,
		}
	}
	return result, nil
}

func (a *interviewDataAdapter) GetAnswerCount(ctx context.Context, sessionCode string) (int, error) {
	return a.store.GetAnswerCount(ctx, sessionCode)
}

// sessionDataAdapter adapts session.PostgresStore to report.SessionProvider.
type sessionDataAdapter struct {
	store *session.PostgresStore
}

func (a *sessionDataAdapter) GetSessionByCode(ctx context.Context, sessionCode string) (*report.SessionInfo, error) {
	sess, err := a.store.GetSessionByCode(ctx, sessionCode)
	if err != nil {
		return nil, err
	}
	info := &report.SessionInfo{
		SessionCode: sess.SessionCode,
		Status:      sess.Status,
	}
	if sess.InterviewStartedAt != nil {
		info.InterviewStartedAt = sess.InterviewStartedAt.Unix()
	}
	if sess.EndedAt != nil {
		info.EndedAt = sess.EndedAt.Unix()
	}
	return info, nil
}
