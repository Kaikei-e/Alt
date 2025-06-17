package logger

import (
	"log/slog"
	"os"
)

var Logger *slog.Logger

func Init() {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})

	Logger = slog.New(handler)
	Logger.Info("Logger initialized")
}
