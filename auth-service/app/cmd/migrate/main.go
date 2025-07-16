package main

import (
	"embed"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"github.com/joho/godotenv"

	"auth-service/app/config"
	"auth-service/app/utils/database"
	"auth-service/app/utils/logger"
	"auth-service/app/utils/migration"
)

//go:embed migrations
var migrationsFS embed.FS

func main() {
	var (
		command = flag.String("command", "up", "Migration command (up, down, status)")
		steps   = flag.String("steps", "0", "Number of steps for down migration")
		verbose = flag.Bool("verbose", false, "Enable verbose logging")
	)
	flag.Parse()

	// Load environment variables
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		slog.Warn("Could not load .env file", "error", err)
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Initialize logger
	logLevel := cfg.LogLevel
	if *verbose {
		logLevel = "debug"
	}

	appLogger, err := logger.New(logLevel)
	if err != nil {
		slog.Error("Failed to initialize logger", "error", err)
		os.Exit(1)
	}

	// Create database connection
	dbConfig := &database.Config{
		Host:            cfg.DatabaseHost,
		Port:            parsePort(cfg.DatabasePort),
		User:            cfg.DatabaseUser,
		Password:        cfg.DatabasePassword,
		Database:        cfg.DatabaseName,
		SSLMode:         cfg.DatabaseSSLMode,
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: cfg.SessionTimeout,
		ConnMaxIdleTime: cfg.SessionTimeout / 2,
		ConnTimeout:     cfg.SessionTimeout,
	}

	dbConn, err := database.NewConnection(dbConfig, appLogger)
	if err != nil {
		appLogger.Error("Failed to create database connection", "error", err)
		os.Exit(1)
	}
	defer dbConn.Close()

	// Create migrator
	migrator := migration.NewMigrator(dbConn.DB(), appLogger, migrationsFS)

	// Execute command
	switch *command {
	case "up":
		if err := migrator.Up(); err != nil {
			appLogger.Error("Migration up failed", "error", err)
			os.Exit(1)
		}
		appLogger.Info("All migrations applied successfully")

	case "down":
		stepCount, err := strconv.Atoi(*steps)
		if err != nil {
			appLogger.Error("Invalid steps value", "steps", *steps, "error", err)
			os.Exit(1)
		}

		if stepCount <= 0 {
			stepCount = 1
		}

		for i := 0; i < stepCount; i++ {
			if err := migrator.Down(); err != nil {
				appLogger.Error("Migration down failed", "error", err, "step", i+1)
				os.Exit(1)
			}
		}
		appLogger.Info("Migrations rolled back successfully", "steps", stepCount)

	case "status":
		if err := migrator.Status(); err != nil {
			appLogger.Error("Migration status failed", "error", err)
			os.Exit(1)
		}

	default:
		appLogger.Error("Unknown command", "command", *command)
		fmt.Println("Available commands: up, down, status")
		os.Exit(1)
	}
}

func parsePort(portStr string) int {
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return 5432 // default PostgreSQL port
	}
	return port
}
