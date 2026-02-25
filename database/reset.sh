#!/bin/bash
# reset.sh — dev only: rolls back all migrations then re-applies them
# Usage: ./reset.sh

set -e

echo "rolling back all migrations..."
go run main.go down all

echo "applying all migrations..."
go run main.go up

echo "done"
