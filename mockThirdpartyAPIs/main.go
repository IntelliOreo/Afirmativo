package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

func main() {
	http.HandleFunc("POST /v1/listen", handleDeepgramListen)
	http.HandleFunc("POST /v1/auth/grant", handleDeepgramTokenGrant)
	http.HandleFunc("POST /v1/auth/tokens/grant", handleDeepgramTokenGrant)

	fmt.Println("Mock Deepgram API server running on :9090")
	fmt.Println("  POST /v1/listen — Deepgram pre-recorded listen API")
	fmt.Println("  POST /v1/auth/grant — Deepgram token grant API")
	fmt.Println("  POST /v1/auth/tokens/grant — Deepgram token grant API (alias)")
	http.ListenAndServe(":9090", nil)
}

func handleDeepgramListen(w http.ResponseWriter, r *http.Request) {
	_ = r.Header.Get("Authorization") // Accept any token as valid.

	resp := map[string]interface{}{
		"metadata": map[string]interface{}{
			"request_id": "mock-request-id",
			"created":    time.Now().UTC().Format(time.RFC3339),
			"duration":   0.0,
			"channels":   1,
		},
		"results": map[string]interface{}{
			"channels": []map[string]interface{}{
				{
					"alternatives": []map[string]interface{}{
						{
							"transcript": "mock response",
							"confidence": 1.0,
						},
					},
				},
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func handleDeepgramTokenGrant(w http.ResponseWriter, r *http.Request) {
	_ = r.Header.Get("Authorization") // Accept any token as valid.

	resp := map[string]interface{}{
		"access_token": fmt.Sprintf("%d", time.Now().Unix()),
		"token_type":   "Bearer",
		"expires_in":   3600,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
