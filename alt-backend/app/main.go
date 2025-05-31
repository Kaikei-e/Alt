package main

import (
	"alt/di"
	"alt/driver/alt_db"
	"alt/job"
	"alt/rest"
	"alt/utils/logger"
	"context"

	"github.com/labstack/echo/v4"
)

func main() {
	ctx := context.Background()

	log := logger.InitLogger()
	log.Info("Starting server")

	db, err := alt_db.InitDBConnection(ctx)
	if err != nil {
		logger.Logger.Error("Failed to connect to database", "error", err)
		panic(err)
	}
	defer db.Close(context.Background())

	container := di.NewApplicationComponents(db)

	job.HourlyJobRunner(ctx)

	e := echo.New()
	rest.RegisterRoutes(e, container)
	err = e.Start(":9000")
	if err != nil {
		logger.Logger.Error("Error starting server", "error", err)
		panic(err)
	}
}
