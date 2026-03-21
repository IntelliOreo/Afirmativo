package main

// Version is the application version.
// Default value is set here. Can be overridden at build time via:
//
//	go build -ldflags="-X main.Version=1.2.3" ./cmd/server
var Version = "0.1.0"
