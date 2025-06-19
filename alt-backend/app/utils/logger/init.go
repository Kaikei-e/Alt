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

// SafeInfo logs an info message if the logger is initialized, otherwise does nothing
func SafeInfo(msg string, args ...any) {
	if Logger != nil {
		Logger.Info(msg, args...)
	}
}

// SafeError logs an error message if the logger is initialized, otherwise does nothing
func SafeError(msg string, args ...any) {
	if Logger != nil {
		Logger.Error(msg, args...)
	}
}

// SafeWarn logs a warning message if the logger is initialized, otherwise does nothing
func SafeWarn(msg string, args ...any) {
	if Logger != nil {
		Logger.Warn(msg, args...)
	}
}
