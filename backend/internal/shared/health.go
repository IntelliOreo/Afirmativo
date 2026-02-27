// Health check handler: GET /api/health.
// Pings the database to wake Neon and confirm connectivity.
// Returns {"status":"ok","db":"connected"} or 503 on failure.
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

// HandleHealth returns a health check handler that pings the database.
func HandleHealth(db DBPinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		if err := db.Ping(ctx); err != nil {
			WriteJSON(w, http.StatusServiceUnavailable, map[string]string{
				"status": "error",
				"db":     "unreachable",
			})
			return
		}

		WriteJSON(w, http.StatusOK, map[string]string{
			"status": "ok",
			"db":     "connected",
		})
	}
}
