package interview

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/afirmativo/backend/internal/vertexai"
)

func TestVertexInterviewAIClientGenerateTurn_EvaluationTurnUsesImplicitCachingOnly(t *testing.T) {
	var generateBody map[string]any

	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case strings.HasSuffix(r.URL.Path, ":generateContent"):
			if err := json.NewDecoder(r.Body).Decode(&generateBody); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
			return jsonResponse(http.StatusOK, map[string]any{
				"candidates": []map[string]any{
					{
						"content": map[string]any{
							"parts": []map[string]any{{"text": `{"evaluation":{"current_criterion":{"id":1,"status":"sufficient","evidence_summary":"clear evidence","recommendation":"move_on"},"other_criteria_addressed":[]},"next_question":"Cuenteme mas."}`}},
						},
						"finishReason": "STOP",
					},
				},
				"usageMetadata": map[string]any{
					"promptTokenCount":        100,
					"candidatesTokenCount":    20,
					"totalTokenCount":         120,
					"cachedContentTokenCount": 80,
				},
			}), nil
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
			return nil, nil
		}
	})}

	vertexClient, err := vertexai.NewClient(vertexai.ClientConfig{
		BaseURL:              "https://vertex.test/v1/",
		ProjectID:            "test",
		Location:             "global",
		APIKey:               "vertex-key",
		AuthMode:             vertexai.AuthModeAPIKey,
		HTTPClient:           httpClient,
		TimeoutSeconds:       5,
		ExplicitCacheEnabled: true,
		ContextCacheTTL:      300 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	client := NewVertexInterviewAIClient(VertexInterviewAIClientConfig{
		Model:              "gemini-3.1-flash-lite-preview",
		MaxTokens:          1024,
		SystemPrompt:       strings.Repeat("stable system prompt ", 180),
		PromptLastQuestion: "last question",
		PromptClosing:      "closing",
		PromptOpeningTurn:  "opening",
		VertexClient:       vertexClient,
	})

	resp, err := client.GenerateTurn(context.Background(), &AITurnContext{
		PreferredLanguage:   "es",
		CurrentAreaSlug:     "protected_ground",
		CurrentAreaID:       1,
		CurrentAreaIndex:    0,
		CurrentAreaLabel:    "Protected ground",
		Description:         "Describe the protected ground.",
		SufficiencyReqs:     "Give enough detail.",
		AreaStatus:          AreaStatusInProgress,
		FollowUpsRemaining:  2,
		TimeRemainingS:      600,
		QuestionsRemaining:  10,
		CriteriaRemaining:   5,
		CurrentQuestionText: "Why were you targeted?",
		LatestAnswerText:    "Because of my politics.",
		HistoryTurns: []HistoryTurn{
			{QuestionText: "What happened?", AnswerText: "I was threatened."},
		},
	})
	if err != nil {
		t.Fatalf("GenerateTurn() error = %v", err)
	}
	if resp.NextQuestion != "Cuenteme mas." {
		t.Fatalf("resp.NextQuestion = %q, want Cuenteme mas.", resp.NextQuestion)
	}
	if _, ok := generateBody["cachedContent"]; ok {
		t.Fatalf("cachedContent present for implicit-only interview request; body = %#v", generateBody)
	}
	if _, ok := generateBody["systemInstruction"]; !ok {
		t.Fatalf("systemInstruction missing from implicit-only request")
	}
}

func TestVertexInterviewAIClientGenerateTurn_OpeningTurnUsesImplicitCachingOnly(t *testing.T) {
	var generateBody map[string]any

	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case strings.HasSuffix(r.URL.Path, ":generateContent"):
			if err := json.NewDecoder(r.Body).Decode(&generateBody); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
			return jsonResponse(http.StatusOK, map[string]any{
				"candidates": []map[string]any{
					{
						"content": map[string]any{
							"parts": []map[string]any{{"text": `{"evaluation":null,"next_question":"Please begin."}`}},
						},
						"finishReason": "STOP",
					},
				},
			}), nil
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
			return nil, nil
		}
	})}

	vertexClient, err := vertexai.NewClient(vertexai.ClientConfig{
		BaseURL:              "https://vertex.test/v1/",
		ProjectID:            "test",
		Location:             "global",
		APIKey:               "vertex-key",
		AuthMode:             vertexai.AuthModeAPIKey,
		HTTPClient:           httpClient,
		TimeoutSeconds:       5,
		ExplicitCacheEnabled: true,
		ContextCacheTTL:      300 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	client := NewVertexInterviewAIClient(VertexInterviewAIClientConfig{
		Model:             "gemini-3.1-flash-lite-preview",
		MaxTokens:         1024,
		SystemPrompt:      "small prompt",
		PromptOpeningTurn: "opening",
		VertexClient:      vertexClient,
	})

	resp, err := client.GenerateTurn(context.Background(), &AITurnContext{
		PreferredLanguage:  "en",
		IsOpeningTurn:      true,
		CurrentAreaSlug:    "open_floor",
		CurrentAreaID:      7,
		CurrentAreaIndex:   0,
		CurrentAreaLabel:   "Open floor",
		Description:        "Anything else.",
		SufficiencyReqs:    "Always sufficient.",
		AreaStatus:         AreaStatusInProgress,
		FollowUpsRemaining: 6,
		TimeRemainingS:     300,
		QuestionsRemaining: 5,
		CriteriaRemaining:  1,
	})
	if err != nil {
		t.Fatalf("GenerateTurn() error = %v", err)
	}
	if resp.NextQuestion != "Please begin." {
		t.Fatalf("resp.NextQuestion = %q, want Please begin.", resp.NextQuestion)
	}
	if _, ok := generateBody["cachedContent"]; ok {
		t.Fatalf("cachedContent present for below-threshold prompt; body = %#v", generateBody)
	}
	if _, ok := generateBody["systemInstruction"]; !ok {
		t.Fatalf("systemInstruction missing from implicit-cache request")
	}
}

func stringValue(value any) string {
	got, _ := value.(string)
	return got
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func jsonResponse(statusCode int, body any) *http.Response {
	raw, _ := json.Marshal(body)
	return &http.Response{
		StatusCode: statusCode,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(string(raw))),
	}
}
