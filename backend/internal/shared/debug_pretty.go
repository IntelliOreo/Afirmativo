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
