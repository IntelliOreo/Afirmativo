package main

import (
	"github.com/afirmativo/backend/internal/config"
	"github.com/afirmativo/backend/internal/interview"
	"github.com/afirmativo/backend/internal/report"
)

func createAIClients(cfg config.Config) (interview.InterviewAIClient, report.ReportAIClient) {
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
			})
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
		})
}
