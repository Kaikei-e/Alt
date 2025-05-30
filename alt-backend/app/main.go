package main

import (
	"alt/job"
	"alt/rest"
	"alt/utils/logger"
	"context"

	"github.com/labstack/echo/v4"
)

func main() {
	logger := logger.InitLogger()
	logger.Info("Starting server")

	ctx := context.Background()
	job.HourlyJobRunner(ctx)

	e := echo.New()
	rest.RegisterRoutes(e)
	err := e.Start(":9000")
	if err != nil {
		logger.Error("Error starting server", "error", err)
		panic(err)
	}
}
