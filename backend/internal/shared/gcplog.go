// GCP Cloud Logging compatible slog handler factory.
// Remaps slog severity levels to the "severity" field expected by Cloud Logging
// and renames "msg" to "message" for proper ingestion.
package shared

import (
	"io"
	"log/slog"
)

// NewGCPJSONHandler creates a slog.JSONHandler with GCP Cloud Logging-compatible
// field mapping: level → severity (string), msg → message.
func NewGCPJSONHandler(w io.Writer, opts *slog.HandlerOptions) slog.Handler {
	if opts == nil {
		opts = &slog.HandlerOptions{}
	}

	original := opts.ReplaceAttr
	opts.ReplaceAttr = func(groups []string, a slog.Attr) slog.Attr {
		// Map slog level to GCP severity string.
		if a.Key == slog.LevelKey {
			a.Key = "severity"
			level := a.Value.Any().(slog.Level)
			switch {
			case level < slog.LevelInfo:
				a.Value = slog.StringValue("DEBUG")
			case level < slog.LevelWarn:
				a.Value = slog.StringValue("INFO")
			case level < slog.LevelError:
				a.Value = slog.StringValue("WARNING")
			default:
				a.Value = slog.StringValue("ERROR")
			}
		}

		// Cloud Logging expects "message" not "msg".
		if a.Key == slog.MessageKey {
			a.Key = "message"
		}

		if original != nil {
			return original(groups, a)
		}
		return a
	}

	return slog.NewJSONHandler(w, opts)
}
