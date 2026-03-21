// Health check handler: GET /api/health.
// Pings the database to wake Neon and confirm connectivity.
// Optionally includes async runtime stats from registered providers.
package shared

import (
	"context"
	"net/http"
	"time"
)

// DBPinger is satisfied by *pgxpool.Pool.
type DBPinger interface {
	Ping(ctx context.Context) error
}

// HealthStatsProvider supplies runtime stats for the health endpoint.
type HealthStatsProvider interface {
	HealthStats() map[string]any
}

// HandleHealth returns a health check handler that pings the database
// and optionally merges stats from the given providers.
func HandleHealth(db DBPinger, version string, providers ...HealthStatsProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		resp := map[string]any{
			"status":  "ok",
			"version": version,
			"db":      "connected",
		}

		if err := db.Ping(ctx); err != nil {
			resp["status"] = "error"
			resp["db"] = "unreachable"
			WriteJSON(w, http.StatusServiceUnavailable, resp)
			return
		}

		for _, p := range providers {
			for k, v := range p.HealthStats() {
				resp[k] = v
			}
		}

		WriteJSON(w, http.StatusOK, resp)
	}
}
