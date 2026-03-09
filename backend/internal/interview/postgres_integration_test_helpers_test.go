package interview

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const postgresStoreTestDatabaseURLEnv = "AFIRMATIVO_TEST_DATABASE_URL"

type postgresIntegrationSessionParams struct {
	SessionCode           string
	Status                string
	FlowStep              FlowStep
	ExpectedTurnID        string
	DisplayQuestionNumber int
}

type postgresIntegrationAreaParams struct {
	SessionCode          string
	Area                 string
	Status               AreaStatus
	QuestionsCount       int
	PreAddressedEvidence string
}

type postgresIntegrationEvent struct {
	EventType  string
	AnswerText string
}

func newPostgresIntegrationStore(t *testing.T) (*PostgresStore, func()) {
	t.Helper()

	adminURL := strings.TrimSpace(os.Getenv(postgresStoreTestDatabaseURLEnv))
	if adminURL == "" {
		t.Skipf("%s not set; skipping Postgres integration test", postgresStoreTestDatabaseURLEnv)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	adminPool, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		t.Fatalf("pgxpool.New(admin) error = %v", err)
	}
	if err := adminPool.Ping(ctx); err != nil {
		adminPool.Close()
		t.Fatalf("adminPool.Ping() error = %v", err)
	}

	testDBName := "afirmativo_test_" + randomPostgresIntegrationTestSuffix(t)
	if _, err := adminPool.Exec(ctx, fmt.Sprintf("CREATE DATABASE %s", testDBName)); err != nil {
		adminPool.Close()
		t.Fatalf("CREATE DATABASE %s error = %v", testDBName, err)
	}

	testDBURL := withPostgresIntegrationDatabaseName(t, adminURL, testDBName)
	testPool, err := pgxpool.New(ctx, testDBURL)
	if err != nil {
		dropPostgresIntegrationTestDatabase(t, adminPool, testDBName)
		adminPool.Close()
		t.Fatalf("pgxpool.New(test) error = %v", err)
	}
	if err := testPool.Ping(ctx); err != nil {
		testPool.Close()
		dropPostgresIntegrationTestDatabase(t, adminPool, testDBName)
		adminPool.Close()
		t.Fatalf("testPool.Ping() error = %v", err)
	}

	if _, err := testPool.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS pgcrypto`); err != nil {
		testPool.Close()
		dropPostgresIntegrationTestDatabase(t, adminPool, testDBName)
		adminPool.Close()
		t.Fatalf("CREATE EXTENSION pgcrypto error = %v", err)
	}

	applyPostgresIntegrationTestMigrations(t, testPool)

	cleanup := func() {
		testPool.Close()
		dropPostgresIntegrationTestDatabase(t, adminPool, testDBName)
		adminPool.Close()
	}

	return NewPostgresStore(testPool), cleanup
}

func dropPostgresIntegrationTestDatabase(t *testing.T, adminPool *pgxpool.Pool, testDBName string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if _, err := adminPool.Exec(ctx,
		`SELECT pg_terminate_backend(pid)
		   FROM pg_stat_activity
		  WHERE datname = $1
		    AND pid <> pg_backend_pid()`,
		testDBName,
	); err != nil {
		t.Fatalf("pg_terminate_backend(%s) error = %v", testDBName, err)
	}

	if _, err := adminPool.Exec(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s", testDBName)); err != nil {
		t.Fatalf("DROP DATABASE %s error = %v", testDBName, err)
	}
}

func applyPostgresIntegrationTestMigrations(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for _, migrationPath := range postgresIntegrationMigrationPaths(t) {
		contents, err := os.ReadFile(migrationPath)
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", migrationPath, err)
		}

		for _, statement := range splitPostgresIntegrationMigrationStatements(string(contents)) {
			if _, err := pool.Exec(ctx, statement); err != nil {
				t.Fatalf("apply migration %s error = %v", filepath.Base(migrationPath), err)
			}
		}
	}
}

func postgresIntegrationMigrationPaths(t *testing.T) []string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}

	migrationsDir := filepath.Join(filepath.Dir(currentFile), "..", "..", "..", "database", "migrations")
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatalf("ReadDir(%s) error = %v", migrationsDir, err)
	}

	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".up.sql") {
			continue
		}
		paths = append(paths, filepath.Join(migrationsDir, entry.Name()))
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		t.Fatalf("no .up.sql migrations found in %s", migrationsDir)
	}

	return paths
}

func splitPostgresIntegrationMigrationStatements(contents string) []string {
	sanitized := stripPostgresIntegrationMigrationCommentLines(contents)
	parts := strings.Split(sanitized, ";")
	statements := make([]string, 0, len(parts))
	for _, part := range parts {
		statement := strings.TrimSpace(part)
		if statement == "" {
			continue
		}
		statements = append(statements, statement)
	}
	return statements
}

func stripPostgresIntegrationMigrationCommentLines(contents string) string {
	lines := strings.Split(contents, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

func insertPostgresIntegrationSession(t *testing.T, pool *pgxpool.Pool, params postgresIntegrationSessionParams) {
	t.Helper()

	status := strings.TrimSpace(params.Status)
	if status == "" {
		status = "created"
	}

	flowStep := params.FlowStep
	if flowStep == "" {
		flowStep = FlowStepDisclaimer
	}

	displayQuestionNumber := params.DisplayQuestionNumber
	if displayQuestionNumber <= 0 {
		displayQuestionNumber = 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := pool.Exec(ctx,
		`INSERT INTO sessions (
		     session_code,
		     pin_hash,
		     expires_at,
		     status,
		     flow_step,
		     expected_turn_id,
		     display_question_number
		 )
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		params.SessionCode,
		"pin-hash",
		time.Now().UTC().Add(24*time.Hour),
		status,
		string(flowStep),
		nullIfEmpty(params.ExpectedTurnID),
		displayQuestionNumber,
	); err != nil {
		t.Fatalf("insert session %s error = %v", params.SessionCode, err)
	}
}

func insertPostgresIntegrationArea(t *testing.T, pool *pgxpool.Pool, params postgresIntegrationAreaParams) {
	t.Helper()

	status := params.Status
	if status == "" {
		status = AreaStatusPending
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := pool.Exec(ctx,
		`INSERT INTO question_areas (
		     session_code,
		     area,
		     status,
		     questions_count,
		     pre_addressed_evidence
		 )
		 VALUES ($1, $2, $3, $4, $5)`,
		params.SessionCode,
		params.Area,
		string(status),
		params.QuestionsCount,
		pgtype.Text{String: params.PreAddressedEvidence, Valid: strings.TrimSpace(params.PreAddressedEvidence) != ""},
	); err != nil {
		t.Fatalf("insert question area %s/%s error = %v", params.SessionCode, params.Area, err)
	}
}

func loadPostgresIntegrationEvents(t *testing.T, pool *pgxpool.Pool, sessionCode string) []postgresIntegrationEvent {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx,
		`SELECT event_type, COALESCE(answer_text, '')
		   FROM interview_events
		  WHERE session_code = $1
		  ORDER BY created_at ASC`,
		sessionCode,
	)
	if err != nil {
		t.Fatalf("query interview events for %s error = %v", sessionCode, err)
	}
	defer rows.Close()

	var events []postgresIntegrationEvent
	for rows.Next() {
		var event postgresIntegrationEvent
		if err := rows.Scan(&event.EventType, &event.AnswerText); err != nil {
			t.Fatalf("scan interview event for %s error = %v", sessionCode, err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate interview events for %s error = %v", sessionCode, err)
	}
	return events
}

func mustGetPostgresIntegrationArea(t *testing.T, areas []QuestionArea, slug string) QuestionArea {
	t.Helper()

	for _, area := range areas {
		if area.Area == slug {
			return area
		}
	}
	t.Fatalf("area %q not found", slug)
	return QuestionArea{}
}

func mustGetPostgresIntegrationEndedAt(t *testing.T, pool *pgxpool.Pool, sessionCode, area string) *time.Time {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var endedAt pgtype.Timestamptz
	if err := pool.QueryRow(ctx,
		`SELECT area_ended_at
		   FROM question_areas
		  WHERE session_code = $1
		    AND area = $2`,
		sessionCode,
		area,
	).Scan(&endedAt); err != nil {
		t.Fatalf("query area_ended_at for %s/%s error = %v", sessionCode, area, err)
	}
	if !endedAt.Valid {
		return nil
	}
	return &endedAt.Time
}

func mustEqualPostgresIntegrationJSON(t *testing.T, got, want []byte) {
	t.Helper()

	if !equalPostgresIntegrationJSON(t, got, want) {
		t.Fatalf("got JSON %s, want JSON-equal %s", got, want)
	}
}

func equalPostgresIntegrationJSON(t *testing.T, got, want []byte) bool {
	t.Helper()

	var gotValue any
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatalf("json.Unmarshal(got) error = %v", err)
	}

	var wantValue any
	if err := json.Unmarshal(want, &wantValue); err != nil {
		t.Fatalf("json.Unmarshal(want) error = %v", err)
	}

	gotCanonical, err := json.Marshal(gotValue)
	if err != nil {
		t.Fatalf("json.Marshal(got) error = %v", err)
	}
	wantCanonical, err := json.Marshal(wantValue)
	if err != nil {
		t.Fatalf("json.Marshal(want) error = %v", err)
	}

	return string(gotCanonical) == string(wantCanonical)
}

func withPostgresIntegrationDatabaseName(t *testing.T, rawURL, databaseName string) string {
	t.Helper()

	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("url.Parse(%q) error = %v", rawURL, err)
	}
	parsed.Path = "/" + databaseName
	return parsed.String()
}

func randomPostgresIntegrationTestSuffix(t *testing.T) string {
	t.Helper()

	var raw [4]byte
	if _, err := rand.Read(raw[:]); err != nil {
		t.Fatalf("rand.Read() error = %v", err)
	}
	return hex.EncodeToString(raw[:])
}

func TestSplitPostgresIntegrationMigrationStatementsSkipsCommentOnlyChunks(t *testing.T) {
	contents := `-- Migration 000004: Add pre_addressed_evidence to question_areas, change default status to pending.

-- Add column for storing AI reasoning when a criterion is flagged as covered elsewhere.
ALTER TABLE question_areas ADD COLUMN pre_addressed_evidence TEXT;

-- Change default status from 'in_progress' to 'pending'.
-- New rows start as pending; the backend explicitly sets in_progress on the current criterion.
ALTER TABLE question_areas ALTER COLUMN status SET DEFAULT 'pending';`

	got := splitPostgresIntegrationMigrationStatements(contents)
	if len(got) != 2 {
		t.Fatalf("len(splitPostgresIntegrationMigrationStatements()) = %d, want 2", len(got))
	}
	if got[0] != "ALTER TABLE question_areas ADD COLUMN pre_addressed_evidence TEXT" {
		t.Fatalf("first statement = %q", got[0])
	}
	if got[1] != "ALTER TABLE question_areas ALTER COLUMN status SET DEFAULT 'pending'" {
		t.Fatalf("second statement = %q", got[1])
	}
}

func assertPostgresIntegrationConflict(t *testing.T, err error) {
	t.Helper()

	if !errors.Is(err, ErrTurnConflict) {
		t.Fatalf("error = %v, want %v", err, ErrTurnConflict)
	}
}
