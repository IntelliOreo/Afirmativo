package main

import (
	"fmt"
	"log"
	"os"

	"github.com/afirmativo/database/cmd"
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

	command := "up"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}

	// args after the command name (e.g., "down all" → args = ["all"])
	args := os.Args[2:]

	switch command {
	case "up":
		cmd.MigrateUp(dbURL)

	case "down":
		cmd.MigrateDown(dbURL, args)

	case "version":
		cmd.MigrateVersion(dbURL)

	case "load_coupon":
		cmd.LoadCoupon(dbURL, args)

	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "usage: go run main.go <command> [args]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "commands:")
	fmt.Fprintln(os.Stderr, "  up                 apply all pending migrations")
	fmt.Fprintln(os.Stderr, "  down               roll back 1 migration")
	fmt.Fprintln(os.Stderr, "  down all           roll back all migrations")
	fmt.Fprintln(os.Stderr, "  down <N>           roll back N migrations")
	fmt.Fprintln(os.Stderr, "  version            show current migration version")
	fmt.Fprintln(os.Stderr, "  load_coupon        generate and load coupons into DB")
}
