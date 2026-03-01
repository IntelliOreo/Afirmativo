// HTTP client that calls the AI API to get the next interview question.
// Currently points to the mock server; will be updated for real AI inference.
package interview

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// HTTPAIClient implements AIClient by calling an external HTTP API.
type HTTPAIClient struct {
	baseURL string
	client  *http.Client
}

// NewHTTPAIClient creates a client that calls the AI API at the given base URL.
func NewHTTPAIClient(baseURL string) *HTTPAIClient {
	return &HTTPAIClient{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// NextQuestion calls POST /api on the AI server and returns the next question.
func (c *HTTPAIClient) NextQuestion(ctx context.Context, questionNumber int) (*Question, error) {
	url := c.baseURL + "/api"
	slog.Debug("calling AI API", "url", url, "question_number", questionNumber)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build AI request: %w", err)
	}

	start := time.Now()
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call AI API: %w", err)
	}
	defer resp.Body.Close()

	slog.Debug("AI API responded", "status", resp.StatusCode, "duration", time.Since(start), "question_number", questionNumber)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("AI API returned status %d", resp.StatusCode)
	}

	var body struct {
		Question string `json:"question"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode AI response: %w", err)
	}

	return &Question{
		TextEs:         body.Question,
		TextEn:         body.Question,
		Area:           DefinedAreas[0].Slug,
		QuestionNumber: questionNumber,
		TotalQuestions: EstimatedTotalQuestions,
	}, nil
}
