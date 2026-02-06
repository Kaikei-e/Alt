package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"pre-processor/bootstrap"
	logger "pre-processor/utils/logger"
)

func performHealthCheck() {
	port := os.Getenv("HTTP_PORT")
	if port == "" {
		port = "9200"
	}
	rawURL := fmt.Sprintf("http://localhost:%s/api/v1/health", port)

	logger.Logger.Info("Performing health check", "url", rawURL)

	urlParsed, err := url.Parse(rawURL)
	if err != nil {
		logger.Logger.Error("Failed to parse URL", "error", err)
		panic(err)
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get(urlParsed.String())
	if err != nil {
		os.Exit(1)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			logger.Logger.Warn("health check: failed to close response body", "error", cerr, "url", rawURL)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		os.Exit(1)
	}
	os.Exit(0)
}

func main() {
	// Support both --health-check flag and healthcheck subcommand
	if len(os.Args) > 1 && (os.Args[1] == "--health-check" || os.Args[1] == "healthcheck") {
		performHealthCheck()
		return
	}

	if err := bootstrap.Run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}
