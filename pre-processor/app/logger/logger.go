package logger

import (
	"log/slog"
	"os"
)

var Logger *slog.Logger

func Init() *slog.Logger {
	Logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	slog.SetDefault(Logger)

	return Logger
}
