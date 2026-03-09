package shared

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
)

// DebugJSON prints a readable multiline JSON block when debug logging is enabled.
func DebugJSON(label string, payload any) {
	if !slog.Default().Enabled(context.Background(), slog.LevelDebug) {
		return
	}

	pretty, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		slog.Debug(label, "payload", payload, "pretty_error", err)
		return
	}

	fmt.Fprintf(os.Stdout, "level=DEBUG msg=%q\n%s\n", label, pretty)
}

// DebugJSONText pretty-prints a raw JSON string when debug logging is enabled.
func DebugJSONText(label, raw string) {
	if !slog.Default().Enabled(context.Background(), slog.LevelDebug) {
		return
	}

	var parsed any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		fmt.Fprintf(os.Stdout, "level=DEBUG msg=%q\n%s\n", label, raw)
		return
	}

	DebugJSON(label, parsed)
}

// DebugTextBlock prints a readable multiline text block when debug logging is enabled.
func DebugTextBlock(label, text string) {
	if !slog.Default().Enabled(context.Background(), slog.LevelDebug) {
		return
	}

	fmt.Fprintf(os.Stdout, "level=DEBUG msg=%q\n%s\n", label, text)
}

// DebugChatMessages prints chat message role + raw multiline content for readability.
func DebugChatMessages(label string, messages []map[string]interface{}) {
	if !slog.Default().Enabled(context.Background(), slog.LevelDebug) {
		return
	}

	for i, msg := range messages {
		role, _ := msg["role"].(string)
		fmt.Fprintf(os.Stdout, "level=DEBUG msg=%q message_index=%d role=%q\n", label, i, role)
		switch content := msg["content"].(type) {
		case string:
			fmt.Fprintf(os.Stdout, "%s\n", content)
		default:
			pretty, err := json.MarshalIndent(content, "", "  ")
			if err != nil {
				fmt.Fprintf(os.Stdout, "%v\n", content)
				continue
			}
			fmt.Fprintf(os.Stdout, "%s\n", pretty)
		}
	}
}
