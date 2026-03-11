package main

import (
	"fmt"
	"time"

	"github.com/afirmativo/backend/internal/config"
	"github.com/afirmativo/backend/internal/interview"
	"github.com/afirmativo/backend/internal/report"
	"github.com/afirmativo/backend/internal/vertexai"
)

func createAIClients(cfg config.Config) (interview.InterviewAIClient, report.ReportAIClient, error) {
	if cfg.AIProvider == "ollama" {
		return interview.NewOllamaInterviewAIClient(interview.OllamaInterviewAIClientConfig{
				BaseURL:                 cfg.OllamaBaseURL,
				Model:                   cfg.AIModel,
				MaxTokens:               cfg.AIMaxTokens,
				Temperature:             cfg.OllamaTemperature,
				AllowSensitiveDebugLogs: cfg.AllowSensitiveDebugLogs,
				SystemPrompt:            cfg.AIInterviewSystemPrompt,
				OutputFormatPrompt:      cfg.UnstructuredInterviewOutputFormatPrompt,
				PromptLastQuestion:      cfg.AIInterviewPromptLastQuestion,
				PromptClosing:           cfg.AIInterviewPromptClosing,
				PromptOpeningTurn:       cfg.AIInterviewPromptOpeningTurn,
				LastQuestionSeconds:     cfg.AIInterviewLastQuestionSeconds,
				ClosingSeconds:          cfg.AIInterviewClosingSeconds,
				MidpointAreaIndex:       cfg.AIInterviewMidpointAreaIndex,
				TimeoutSeconds:          cfg.AITimeoutSeconds,
				AreaConfigs:             cfg.AreaConfigs,
			}), report.NewOllamaReportAIClient(report.OllamaReportAIClientConfig{
				BaseURL:                 cfg.OllamaBaseURL,
				Model:                   cfg.AIModel,
				MaxTokens:               cfg.AIReportMaxTokens,
				Temperature:             cfg.OllamaTemperature,
				TimeoutSeconds:          cfg.AITimeoutSeconds,
				ReportPrompt:            cfg.AIReportPrompt,
				OutputFormatPrompt:      cfg.UnstructuredReportOutputFormatPrompt,
				AllowSensitiveDebugLogs: cfg.AllowSensitiveDebugLogs,
			}), nil
	}

	if cfg.AIProvider == "vertex" {
		vertexClient, err := vertexai.NewClient(vertexai.ClientConfig{
			ProjectID:            cfg.VertexAIProjectID,
			Location:             cfg.VertexAILocation,
			APIKey:               cfg.VertexAIAPIKey,
			AuthMode:             cfg.VertexAIAuthMode,
			TimeoutSeconds:       cfg.AITimeoutSeconds,
			ExplicitCacheEnabled: cfg.VertexAIExplicitCacheEnabled,
			ContextCacheTTL:      time.Duration(cfg.VertexAIContextCacheTTLSeconds) * time.Second,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("create Vertex AI client: %w", err)
		}
		return interview.NewVertexInterviewAIClient(interview.VertexInterviewAIClientConfig{
				Model:                   cfg.AIModel,
				MaxTokens:               cfg.AIMaxTokens,
				AllowSensitiveDebugLogs: cfg.AllowSensitiveDebugLogs,
				SystemPrompt:            cfg.AIInterviewSystemPrompt,
				PromptLastQuestion:      cfg.AIInterviewPromptLastQuestion,
				PromptClosing:           cfg.AIInterviewPromptClosing,
				PromptOpeningTurn:       cfg.AIInterviewPromptOpeningTurn,
				LastQuestionSeconds:     cfg.AIInterviewLastQuestionSeconds,
				ClosingSeconds:          cfg.AIInterviewClosingSeconds,
				MidpointAreaIndex:       cfg.AIInterviewMidpointAreaIndex,
				AreaConfigs:             cfg.AreaConfigs,
				VertexClient:            vertexClient,
			}), report.NewVertexReportAIClient(report.VertexReportAIClientConfig{
				Model:                   cfg.AIModel,
				MaxTokens:               cfg.AIReportMaxTokens,
				ReportPrompt:            cfg.AIReportPrompt,
				AllowSensitiveDebugLogs: cfg.AllowSensitiveDebugLogs,
				VertexClient:            vertexClient,
			}), nil
	}

	aiBaseURL := "https://api.anthropic.com"
	if cfg.MockAPIURL != "" {
		aiBaseURL = cfg.MockAPIURL
	}

	return interview.NewHTTPInterviewAIClient(interview.InterviewAIClientConfig{
			BaseURL:                 aiBaseURL,
			APIKey:                  cfg.AIAPIKey,
			Model:                   cfg.AIModel,
			MaxTokens:               cfg.AIMaxTokens,
			AllowSensitiveDebugLogs: cfg.AllowSensitiveDebugLogs,
			SystemPrompt:            cfg.AIInterviewSystemPrompt,
			PromptLastQuestion:      cfg.AIInterviewPromptLastQuestion,
			PromptClosing:           cfg.AIInterviewPromptClosing,
			PromptOpeningTurn:       cfg.AIInterviewPromptOpeningTurn,
			LastQuestionSeconds:     cfg.AIInterviewLastQuestionSeconds,
			ClosingSeconds:          cfg.AIInterviewClosingSeconds,
			MidpointAreaIndex:       cfg.AIInterviewMidpointAreaIndex,
			EnablePromptCaching:     cfg.AIInterviewPromptCachingEnabled,
			TimeoutSeconds:          cfg.AITimeoutSeconds,
			AreaConfigs:             cfg.AreaConfigs,
		}), report.NewHTTPReportAIClient(report.ReportAIClientConfig{
			BaseURL:                 aiBaseURL,
			APIKey:                  cfg.AIAPIKey,
			Model:                   cfg.AIModel,
			MaxTokens:               cfg.AIReportMaxTokens,
			TimeoutSeconds:          cfg.AITimeoutSeconds,
			ReportPrompt:            cfg.AIReportPrompt,
			AllowSensitiveDebugLogs: cfg.AllowSensitiveDebugLogs,
		}), nil
}
