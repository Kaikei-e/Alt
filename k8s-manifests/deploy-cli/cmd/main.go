package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	
	"deploy-cli/rest"
	"deploy-cli/utils/logger"
	"deploy-cli/utils/colors"
)

func main() {
	// Create context that can be cancelled
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	
	go func() {
		sig := <-sigChan
		fmt.Printf("\n%s Received signal: %s. Shutting down...\n", colors.Yellow("⚠"), sig)
		cancel()
	}()
	
	// Create logger
	log := logger.NewLogger()
	
	// Create and execute CLI
	cli := rest.NewCLI(log)
	
	if err := cli.Execute(ctx); err != nil {
		log.ErrorWithContext("CLI execution failed", "error", err)
		colors.PrintError(fmt.Sprintf("CLI execution failed: %v", err))
		os.Exit(1)
	}
}