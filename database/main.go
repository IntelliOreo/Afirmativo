package main

import (
	"fmt"
	"log"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Fatal("error loading .env file")
	}

	dbURL := os.Getenv("DATABASE_URL_DIRECT")
	if dbURL == "" {
		log.Fatal("DATABASE_URL_DIRECT not set in .env")
	}

	m, err := migrate.New("file://migrations", dbURL)
	if err != nil {
		log.Fatalf("failed to create migrator: %v", err)
	}
	defer m.Close()

	cmd := "up"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	switch cmd {
	case "up":
		if err := m.Up(); err != nil && err != migrate.ErrNoChange {
			log.Fatalf("migrate up failed: %v", err)
		}
		fmt.Println("migrations applied")

	case "down":
		steps := 1
		if len(os.Args) > 2 && os.Args[2] == "all" {
			if err := m.Down(); err != nil && err != migrate.ErrNoChange {
				log.Fatalf("migrate down failed: %v", err)
			}
			fmt.Println("all migrations rolled back")
			return
		}
		if err := m.Steps(-steps); err != nil && err != migrate.ErrNoChange {
			log.Fatalf("migrate down failed: %v", err)
		}
		fmt.Printf("rolled back %d migration(s)\n", steps)

	case "version":
		version, dirty, err := m.Version()
		if err != nil {
			log.Fatalf("could not get version: %v", err)
		}
		fmt.Printf("version: %d, dirty: %v\n", version, dirty)

	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		fmt.Fprintln(os.Stderr, "usage: go run main.go [up|down|down all|version]")
		os.Exit(1)
	}
}
