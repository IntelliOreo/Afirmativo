package main

import (
	"testing"
	"time"

	"github.com/afirmativo/backend/internal/config"
	"github.com/afirmativo/backend/internal/interview"
	"github.com/afirmativo/backend/internal/report"
)

func TestCreateAIClients_ReturnsVertexClients(t *testing.T) {
	interviewClient, reportClient, err := createAIClients(config.Config{
		Server: config.ServerConfig{
			AllowSensitiveDebugLogs: false,
		},
		Interview: config.InterviewConfig{
			AreaConfigs: []config.AreaConfig{{ID: 1, Slug: "history", Label: "History"}},
		},
		AI: config.AIConfig{
			Provider:                   "vertex",
			Model:                      "gemini-3.1-flash-lite-preview",
			MaxTokens:                  1024,
			ReportMaxTokens:            2048,
			Timeout:                    5 * time.Second,
			VertexAuthMode:             "api_key",
			VertexAPIKey:               "vertex-test-key",
			VertexProjectID:            "afirmativo-dev",
			VertexLocation:             "global",
			VertexExplicitCacheEnabled: true,
			VertexContextCacheTTL:      300 * time.Second,
		},
	})
	if err != nil {
		t.Fatalf("createAIClients() error = %v", err)
	}

	if _, ok := interviewClient.(*interview.VertexInterviewAIClient); !ok {
		t.Fatalf("interviewClient = %T, want *interview.VertexInterviewAIClient", interviewClient)
	}
	if _, ok := reportClient.(*report.VertexReportAIClient); !ok {
		t.Fatalf("reportClient = %T, want *report.VertexReportAIClient", reportClient)
	}
}
