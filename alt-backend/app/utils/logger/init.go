package logger

import (
	"log/slog"
	"os"
)

var Logger *slog.Logger

func InitLogger() *slog.Logger {
	config := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}

	Logger = slog.New(slog.NewTextHandler(os.Stdout, config))
	slog.SetDefault(Logger)

	Logger.Info("Logger initialized")

	return Logger
}
