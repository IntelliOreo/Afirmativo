package main

import (
	"testing"

	"github.com/afirmativo/backend/internal/config"
	"github.com/afirmativo/backend/internal/interview"
	"github.com/afirmativo/backend/internal/report"
)

func TestCreateAIClients_ReturnsVertexClients(t *testing.T) {
	interviewClient, reportClient, err := createAIClients(config.Config{
		AIProvider:                     "vertex",
		AIModel:                        "gemini-3.1-flash-lite-preview",
		AIMaxTokens:                    1024,
		AIReportMaxTokens:              2048,
		AITimeoutSeconds:               5,
		VertexAIAuthMode:               "api_key",
		VertexAIAPIKey:                 "vertex-test-key",
		VertexAIProjectID:              "afirmativo-dev",
		VertexAILocation:               "global",
		VertexAIExplicitCacheEnabled:   true,
		VertexAIContextCacheTTLSeconds: 300,
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
