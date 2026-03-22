package report

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const reportPostgresStoreTestDatabaseURLEnv = "AFIRMATIVO_TEST_DATABASE_URL"

func TestPostgresStoreClaimNextQueuedReport_ClaimsOldestQueuedReport(t *testing.T) {
	store, cleanup := newReportPostgresIntegrationStore(t)
	defer cleanup()

	ctx := context.Background()
	insertReportIntegrationSession(t, store.pool, "AP-REPORT-OLDEST-1")
	insertReportIntegrationSession(t, store.pool, "AP-REPORT-OLDEST-2")

	if err := store.CreateReport(ctx, &Report{
		SessionCode:   "AP-REPORT-OLDEST-1",
		Status:        ReportStatusQueued,
		LastRequestID: "request-report-1",
	}); err != nil {
		t.Fatalf("CreateReport(first) error = %v", err)
	}
	if err := store.CreateReport(ctx, &Report{
		SessionCode:   "AP-REPORT-OLDEST-2",
		Status:        ReportStatusQueued,
		LastRequestID: "request-report-2",
	}); err != nil {
		t.Fatalf("CreateReport(second) error = %v", err)
	}

	older := time.Now().UTC().Add(-2 * time.Minute)
	newer := older.Add(time.Minute)
	if _, err := store.pool.Exec(ctx,
		`UPDATE reports SET updated_at = $2 WHERE session_code = $1`,
		"AP-REPORT-OLDEST-1",
		older,
	); err != nil {
		t.Fatalf("force first report ordering error = %v", err)
	}
	if _, err := store.pool.Exec(ctx,
		`UPDATE reports SET updated_at = $2 WHERE session_code = $1`,
		"AP-REPORT-OLDEST-2",
		newer,
	); err != nil {
		t.Fatalf("force second report ordering error = %v", err)
	}

	claimed, err := store.ClaimNextQueuedReport(ctx)
	if err != nil {
		t.Fatalf("ClaimNextQueuedReport() error = %v", err)
	}
	if claimed == nil {
		t.Fatal("ClaimNextQueuedReport() = nil, want claimed report")
	}
	if claimed.SessionCode != "AP-REPORT-OLDEST-1" {
		t.Fatalf("claimed.SessionCode = %q, want AP-REPORT-OLDEST-1", claimed.SessionCode)
	}
	if claimed.Status != ReportStatusRunning {
		t.Fatalf("claimed.Status = %q, want %q", claimed.Status, ReportStatusRunning)
	}
	if claimed.Attempts != 1 {
		t.Fatalf("claimed.Attempts = %d, want 1", claimed.Attempts)
	}
	if claimed.StartedAt == nil {
		t.Fatal("claimed.StartedAt = nil, want non-nil")
	}
	if claimed.LastRequestID != "request-report-1" {
		t.Fatalf("claimed.LastRequestID = %q, want request-report-1", claimed.LastRequestID)
	}
}

func TestPostgresStoreClaimNextQueuedReport_ReturnsNilWhenQueueEmpty(t *testing.T) {
	store, cleanup := newReportPostgresIntegrationStore(t)
	defer cleanup()

	claimed, err := store.ClaimNextQueuedReport(context.Background())
	if err != nil {
		t.Fatalf("ClaimNextQueuedReport() error = %v", err)
	}
	if claimed != nil {
		t.Fatalf("ClaimNextQueuedReport() = %#v, want nil", claimed)
	}
}

func TestPostgresStoreClaimNextQueuedReport_SkipsLockedReport(t *testing.T) {
	store, cleanup := newReportPostgresIntegrationStore(t)
	defer cleanup()

	ctx := context.Background()
	insertReportIntegrationSession(t, store.pool, "AP-REPORT-LOCKED-1")
	insertReportIntegrationSession(t, store.pool, "AP-REPORT-LOCKED-2")

	if err := store.CreateReport(ctx, &Report{SessionCode: "AP-REPORT-LOCKED-1", Status: ReportStatusQueued}); err != nil {
		t.Fatalf("CreateReport(first) error = %v", err)
	}
	if err := store.CreateReport(ctx, &Report{SessionCode: "AP-REPORT-LOCKED-2", Status: ReportStatusQueued}); err != nil {
		t.Fatalf("CreateReport(second) error = %v", err)
	}

	older := time.Now().UTC().Add(-2 * time.Minute)
	newer := older.Add(time.Minute)
	if _, err := store.pool.Exec(ctx,
		`UPDATE reports SET updated_at = $2 WHERE session_code = $1`,
		"AP-REPORT-LOCKED-1",
		older,
	); err != nil {
		t.Fatalf("force first report ordering error = %v", err)
	}
	if _, err := store.pool.Exec(ctx,
		`UPDATE reports SET updated_at = $2 WHERE session_code = $1`,
		"AP-REPORT-LOCKED-2",
		newer,
	); err != nil {
		t.Fatalf("force second report ordering error = %v", err)
	}

	tx, err := store.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`SELECT session_code
		   FROM reports
		  WHERE session_code = $1
		  FOR UPDATE`,
		"AP-REPORT-LOCKED-1",
	); err != nil {
		t.Fatalf("lock first queued report error = %v", err)
	}

	claimed, err := store.ClaimNextQueuedReport(ctx)
	if err != nil {
		t.Fatalf("ClaimNextQueuedReport() error = %v", err)
	}
	if claimed == nil {
		t.Fatal("ClaimNextQueuedReport() = nil, want claimed report")
	}
	if claimed.SessionCode != "AP-REPORT-LOCKED-2" {
		t.Fatalf("claimed.SessionCode = %q, want AP-REPORT-LOCKED-2", claimed.SessionCode)
	}
}

func newReportPostgresIntegrationStore(t *testing.T) (*PostgresStore, func()) {
	t.Helper()

	adminURL := strings.TrimSpace(os.Getenv(reportPostgresStoreTestDatabaseURLEnv))
	if adminURL == "" {
		t.Skipf("%s not set; skipping Postgres integration test", reportPostgresStoreTestDatabaseURLEnv)
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

	testDBName := "afirmativo_report_test_" + randomReportPostgresTestSuffix(t)
	if _, err := adminPool.Exec(ctx, fmt.Sprintf("CREATE DATABASE %s", testDBName)); err != nil {
		adminPool.Close()
		t.Fatalf("CREATE DATABASE %s error = %v", testDBName, err)
	}

	testDBURL := withReportPostgresDatabaseName(t, adminURL, testDBName)
	testPool, err := pgxpool.New(ctx, testDBURL)
	if err != nil {
		dropReportPostgresTestDatabase(t, adminPool, testDBName)
		adminPool.Close()
		t.Fatalf("pgxpool.New(test) error = %v", err)
	}
	if err := testPool.Ping(ctx); err != nil {
		testPool.Close()
		dropReportPostgresTestDatabase(t, adminPool, testDBName)
		adminPool.Close()
		t.Fatalf("testPool.Ping() error = %v", err)
	}

	if _, err := testPool.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS pgcrypto`); err != nil {
		testPool.Close()
		dropReportPostgresTestDatabase(t, adminPool, testDBName)
		adminPool.Close()
		t.Fatalf("CREATE EXTENSION pgcrypto error = %v", err)
	}

	applyReportPostgresIntegrationTestMigrations(t, testPool)

	cleanup := func() {
		testPool.Close()
		dropReportPostgresTestDatabase(t, adminPool, testDBName)
		adminPool.Close()
	}

	return NewPostgresStore(testPool), cleanup
}

func dropReportPostgresTestDatabase(t *testing.T, adminPool *pgxpool.Pool, testDBName string) {
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

func applyReportPostgresIntegrationTestMigrations(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for _, migrationPath := range reportPostgresIntegrationMigrationPaths(t) {
		contents, err := os.ReadFile(migrationPath)
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", migrationPath, err)
		}

		for _, statement := range splitReportPostgresIntegrationMigrationStatements(string(contents)) {
			if _, err := pool.Exec(ctx, statement); err != nil {
				t.Fatalf("apply migration %s error = %v", filepath.Base(migrationPath), err)
			}
		}
	}
}

func reportPostgresIntegrationMigrationPaths(t *testing.T) []string {
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

func splitReportPostgresIntegrationMigrationStatements(contents string) []string {
	lines := strings.Split(contents, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		kept = append(kept, line)
	}

	parts := strings.Split(strings.Join(kept, "\n"), ";")
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

func insertReportIntegrationSession(t *testing.T, pool *pgxpool.Pool, sessionCode string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := pool.Exec(ctx,
		`INSERT INTO sessions (
		     session_code,
		     pin_hash,
		     expires_at,
		     status,
		     interview_budget_seconds,
		     flow_step,
		     display_question_number
		 )
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		sessionCode,
		"pin-hash",
		time.Now().UTC().Add(24*time.Hour),
		"completed",
		2400,
		"done",
		1,
	); err != nil {
		t.Fatalf("insert session %s error = %v", sessionCode, err)
	}
}

func withReportPostgresDatabaseName(t *testing.T, adminURL, dbName string) string {
	t.Helper()

	parsed, err := url.Parse(adminURL)
	if err != nil {
		t.Fatalf("url.Parse(%q) error = %v", adminURL, err)
	}
	parsed.Path = "/" + dbName
	return parsed.String()
}

func randomReportPostgresTestSuffix(t *testing.T) string {
	t.Helper()

	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("rand.Read() error = %v", err)
	}
	return hex.EncodeToString(buf)
}
