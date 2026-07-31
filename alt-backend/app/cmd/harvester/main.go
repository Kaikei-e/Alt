// Command harvester runs alt-backend's scheduled background jobs: feed
// collection, the outbox worker, the OGP image pipeline, scraping-policy
// refresh and outbox pruning.
//
// It serves no API. The one listener it opens carries /health and /metrics on
// loopback so the container has a probe; there is no Echo instance, no
// Connect-RPC mux, and NewHarvesterComponents builds none of the clients that
// would let one be added by accident.
package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"

	"alt/config"
	"alt/di"
	"alt/internal/bootstrap"
	"alt/orchestrator/job"
)

const serviceName = "alt-harvester"

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rt := bootstrap.MustBoot(ctx, bootstrap.Options{
		ServiceName: serviceName,
		RequireDB:   true,
	})
	defer rt.Shutdown(ctx)

	cfg := rt.Cfg
	log := rt.Log

	if err := config.ValidateHarvesterConfig(cfg); err != nil {
		log.ErrorContext(ctx, "harvester config invalid", "error", err)
		os.Exit(1)
	}
	healthAddr, err := config.LoadHarvesterHealthAddr()
	if err != nil {
		log.ErrorContext(ctx, "harvester health listener config invalid", "error", err)
		os.Exit(1)
	}

	container := di.NewHarvesterComponents(rt.Pool, cfg)

	scheduler := job.NewJobScheduler()
	job.RegisterHarvesterJobs(scheduler, container, cfg)
	log.InfoContext(ctx, "harvester.jobs.wiring", "jobs", scheduler.JobNames())
	scheduler.Start(ctx)

	healthSrv := bootstrap.NewServiceServer(healthAddr, newHealthHandler(rt.MetricsHandler), cfg)
	log.InfoContext(ctx, "harvester_listener.wiring",
		"addr", healthAddr,
		"surfaces", "/health,/metrics",
		"control", "loopback_bind_only",
	)

	sup := bootstrap.NewSupervisor(log)
	sup.AddServer("health", healthSrv.ListenAndServe, healthSrv.Shutdown)
	// Registered as a task, so GracefulShutdown drains the running jobs before
	// closing the listener. The root context is cancelled first (below), which
	// is what lets JobScheduler.runJob return instead of waiting out a full
	// interval.
	sup.AddTask("scheduler", scheduler.Shutdown)
	sup.Start(ctx)

	outcome := sup.Wait(ctx)
	bootstrap.LogMemStats(ctx, log)
	cancel()
	sup.GracefulShutdown(ctx, cfg.Server.WriteTimeout)

	log.Info("harvester stopped", "reason", outcome.Reason, "signal", outcome.Signal, "server", outcome.Server)
}

// newHealthHandler builds the harvester's only handler on an explicit mux.
//
// It must not be http.DefaultServeMux: that mux is where net/http/pprof
// registers itself, and serving it here would publish heap and goroutine dumps
// from the health endpoint. alt/internal/profiling owns that import and only
// cmd/backend links it, so the risk is structural rather than a matter of
// remembering — but the explicit mux keeps it that way.
func newHealthHandler(metrics http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "healthy", "service": serviceName})
	})
	if metrics != nil {
		mux.Handle("/metrics", metrics)
	}
	return mux
}
