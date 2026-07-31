// Command harvester runs alt-backend's scheduled background jobs: feed
// collection, the outbox worker, the OGP image pipeline, scraping-policy
// refresh and outbox pruning.
//
// It serves no API. The one listener it opens is the ops listener shared by
// all three binaries — /health for the compose probe, /metrics for Prometheus
// — so the container has something to probe; there is no Echo instance, no
// Connect-RPC mux, and NewHarvesterComponents builds none of the clients that
// would let one be added by accident.
package main

import (
	"context"
	"fmt"
	"os"

	"alt/config"
	"alt/di"
	"alt/internal/bootstrap"
	"alt/orchestrator/job"
)

const serviceName = "alt-harvester"

func main() {
	// Runs before MustBoot: compose probes every 20s, and a probe that opened
	// the database pool first would report a healthy job runner unhealthy
	// whenever Postgres was slow.
	if bootstrap.IsHealthcheckInvocation(os.Args) {
		runHealthcheck()
	}

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
	opsAddr, err := config.LoadOpsListenAddr()
	if err != nil {
		log.ErrorContext(ctx, "ops listener config invalid", "error", err)
		os.Exit(1)
	}

	container := di.NewHarvesterComponents(rt.Pool, cfg)

	scheduler := job.NewJobScheduler()
	job.RegisterHarvesterJobs(scheduler, container, cfg)
	log.InfoContext(ctx, "harvester.jobs.wiring", "jobs", scheduler.JobNames())
	scheduler.Start(ctx)

	bootstrap.LogOpsWiring(ctx, log, opsAddr, rt.MetricsHandler != nil)
	opsSrv := bootstrap.NewOpsServer(opsAddr, bootstrap.NewOpsHandler(serviceName, rt.MetricsHandler), cfg)

	sup := bootstrap.NewSupervisor(log)
	sup.AddServer("ops", opsSrv.ListenAndServe, opsSrv.Shutdown)
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

// runHealthcheck self-probes the ops listener and exits. It is the harvester's
// only socket, so nothing else needs proving: the scheduler runs in-process and
// a dead scheduler goroutine would take the process down with it (the
// supervisor registers it as a task).
func runHealthcheck() {
	addr, err := config.LoadOpsListenAddr()
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: %v\n", err)
		os.Exit(1)
	}
	if err := bootstrap.Healthcheck(context.Background(), bootstrap.HealthcheckOptions{OpsAddr: addr}); err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: %v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}
