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
	if cfg.AI.Provider == "ollama" {
		return interview.NewOllamaInterviewAIClient(interview.OllamaInterviewAIClientConfig{
				BaseURL:                 cfg.AI.OllamaBaseURL,
				Model:                   cfg.AI.Model,
				MaxTokens:               cfg.AI.MaxTokens,
				Temperature:             cfg.AI.OllamaTemperature,
				AllowSensitiveDebugLogs: cfg.Server.AllowSensitiveDebugLogs,
				SystemPrompt:            cfg.AI.InterviewSystemPrompt,
				OutputFormatPrompt:      cfg.AI.UnstructuredInterviewOutputFormatPrompt,
				PromptLastQuestion:      cfg.AI.InterviewPromptLastQuestion,
				PromptClosing:           cfg.AI.InterviewPromptClosing,
				PromptOpeningTurn:       cfg.AI.InterviewPromptOpeningTurn,
				LastQuestionSeconds:     cfg.AI.InterviewLastQuestionSeconds,
				ClosingSeconds:          cfg.AI.InterviewClosingSeconds,
				MidpointAreaIndex:       cfg.AI.InterviewMidpointAreaIndex,
				TimeoutSeconds:          int(cfg.AI.Timeout / time.Second),
				AreaConfigs:             cfg.Interview.AreaConfigs,
			}), report.NewOllamaReportAIClient(report.OllamaReportAIClientConfig{
				BaseURL:                 cfg.AI.OllamaBaseURL,
				Model:                   cfg.AI.Model,
				MaxTokens:               cfg.AI.ReportMaxTokens,
				Temperature:             cfg.AI.OllamaTemperature,
				TimeoutSeconds:          int(cfg.AI.Timeout / time.Second),
				ReportPrompt:            cfg.AI.ReportPrompt,
				OutputFormatPrompt:      cfg.AI.UnstructuredReportOutputFormatPrompt,
				AllowSensitiveDebugLogs: cfg.Server.AllowSensitiveDebugLogs,
			}), nil
	}

	if cfg.AI.Provider == "vertex" {
		vertexClient, err := vertexai.NewClient(vertexai.ClientConfig{
			ProjectID:            cfg.AI.VertexProjectID,
			Location:             cfg.AI.VertexLocation,
			APIKey:               cfg.AI.VertexAPIKey,
			AuthMode:             cfg.AI.VertexAuthMode,
			TimeoutSeconds:       int(cfg.AI.Timeout / time.Second),
			ExplicitCacheEnabled: cfg.AI.VertexExplicitCacheEnabled,
			ContextCacheTTL:      cfg.AI.VertexContextCacheTTL,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("create Vertex AI client: %w", err)
		}
		return interview.NewVertexInterviewAIClient(interview.VertexInterviewAIClientConfig{
				Model:                   cfg.AI.Model,
				MaxTokens:               cfg.AI.MaxTokens,
				AllowSensitiveDebugLogs: cfg.Server.AllowSensitiveDebugLogs,
				SystemPrompt:            cfg.AI.InterviewSystemPrompt,
				PromptLastQuestion:      cfg.AI.InterviewPromptLastQuestion,
				PromptClosing:           cfg.AI.InterviewPromptClosing,
				PromptOpeningTurn:       cfg.AI.InterviewPromptOpeningTurn,
				LastQuestionSeconds:     cfg.AI.InterviewLastQuestionSeconds,
				ClosingSeconds:          cfg.AI.InterviewClosingSeconds,
				MidpointAreaIndex:       cfg.AI.InterviewMidpointAreaIndex,
				AreaConfigs:             cfg.Interview.AreaConfigs,
				VertexClient:            vertexClient,
			}), report.NewVertexReportAIClient(report.VertexReportAIClientConfig{
				Model:                   cfg.AI.Model,
				MaxTokens:               cfg.AI.ReportMaxTokens,
				ReportPrompt:            cfg.AI.ReportPrompt,
				AllowSensitiveDebugLogs: cfg.Server.AllowSensitiveDebugLogs,
				VertexClient:            vertexClient,
			}), nil
	}

	aiBaseURL := "https://api.anthropic.com"
	if cfg.AI.MockAPIURL != "" {
		aiBaseURL = cfg.AI.MockAPIURL
	}

	return interview.NewHTTPInterviewAIClient(interview.InterviewAIClientConfig{
			BaseURL:                 aiBaseURL,
			APIKey:                  cfg.AI.APIKey,
			Model:                   cfg.AI.Model,
			MaxTokens:               cfg.AI.MaxTokens,
			AllowSensitiveDebugLogs: cfg.Server.AllowSensitiveDebugLogs,
			SystemPrompt:            cfg.AI.InterviewSystemPrompt,
			PromptLastQuestion:      cfg.AI.InterviewPromptLastQuestion,
			PromptClosing:           cfg.AI.InterviewPromptClosing,
			PromptOpeningTurn:       cfg.AI.InterviewPromptOpeningTurn,
			LastQuestionSeconds:     cfg.AI.InterviewLastQuestionSeconds,
			ClosingSeconds:          cfg.AI.InterviewClosingSeconds,
			MidpointAreaIndex:       cfg.AI.InterviewMidpointAreaIndex,
			EnablePromptCaching:     cfg.AI.InterviewPromptCachingEnabled,
			TimeoutSeconds:          int(cfg.AI.Timeout / time.Second),
			AreaConfigs:             cfg.Interview.AreaConfigs,
		}), report.NewHTTPReportAIClient(report.ReportAIClientConfig{
			BaseURL:                 aiBaseURL,
			APIKey:                  cfg.AI.APIKey,
			Model:                   cfg.AI.Model,
			MaxTokens:               cfg.AI.ReportMaxTokens,
			TimeoutSeconds:          int(cfg.AI.Timeout / time.Second),
			ReportPrompt:            cfg.AI.ReportPrompt,
			AllowSensitiveDebugLogs: cfg.Server.AllowSensitiveDebugLogs,
		}), nil
}
