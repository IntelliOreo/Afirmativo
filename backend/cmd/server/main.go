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
	"strings"
	"os/signal"
	"syscall"
	"time"

	"github.com/afirmativo/backend/internal/config"
	"github.com/afirmativo/backend/internal/interview"
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
	aiClient := interview.NewHTTPAIClient(cfg.MockAPIURL)
	interviewSvc := interview.NewService(sessionStore, interviewStore, aiClient)
	interviewHandler := interview.NewHandler(interviewSvc)

	// Register routes.
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", shared.HandleHealth(pool))
	mux.HandleFunc("POST /api/coupon/validate", sessionHandler.HandleValidateCoupon)
	mux.HandleFunc("POST /api/session/verify", sessionHandler.HandleVerifySession)
	mux.HandleFunc("POST /api/interview/start", interviewHandler.HandleStart)
	mux.HandleFunc("POST /api/interview/answer", interviewHandler.HandleAnswer)

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
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
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
