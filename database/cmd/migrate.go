package cmd

import (
	"fmt"
	"log"
	"strconv"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func newMigrator(dbURL string) *migrate.Migrate {
	m, err := migrate.New("file://migrations", dbURL)
	if err != nil {
		log.Fatalf("failed to create migrator: %v", err)
	}
	return m
}

func printVersion(m *migrate.Migrate) {
	version, dirty, err := m.Version()
	if err == migrate.ErrNoChange {
		fmt.Println("  no migrations applied")
		return
	}
	if err != nil {
		fmt.Printf("  version: unknown (%v)\n", err)
		return
	}
	fmt.Printf("  version: %d, dirty: %v\n", version, dirty)
}

// MigrateUp applies all pending migrations.
func MigrateUp(dbURL string) {
	m := newMigrator(dbURL)
	defer m.Close()

	fmt.Print("before: ")
	printVersion(m)

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		log.Fatalf("migrate up failed: %v", err)
	}

	fmt.Print("after:  ")
	printVersion(m)
	fmt.Println("migrations applied")
}

// MigrateDown rolls back migrations.
//   - no extra arg:  roll back 1 step
//   - "all":         roll back everything
//   - "<N>":         roll back N steps
func MigrateDown(dbURL string, args []string) {
	m := newMigrator(dbURL)
	defer m.Close()

	fmt.Print("before: ")
	printVersion(m)

	if len(args) > 0 && args[0] == "all" {
		if err := m.Down(); err != nil && err != migrate.ErrNoChange {
			log.Fatalf("migrate down failed: %v", err)
		}
		fmt.Println("all migrations rolled back")
		return
	}

	steps := 1
	if len(args) > 0 {
		n, err := strconv.Atoi(args[0])
		if err != nil || n < 1 {
			log.Fatalf("invalid step count: %s (must be a positive integer or 'all')", args[0])
		}
		steps = n
	}

	if err := m.Steps(-steps); err != nil && err != migrate.ErrNoChange {
		log.Fatalf("migrate down failed: %v", err)
	}

	fmt.Print("after:  ")
	printVersion(m)
	fmt.Printf("rolled back %d migration(s)\n", steps)
}

// MigrateVersion prints the current migration version.
func MigrateVersion(dbURL string) {
	m := newMigrator(dbURL)
	defer m.Close()
	printVersion(m)
}
