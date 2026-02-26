package cmd

import (
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

// OpenDB opens a raw SQL connection to the database.
// Use this for data operations (load_coupon, etc.) — not for migrations.
func OpenDB(dbURL string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open db: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping db: %w", err)
	}
	return db, nil
}
