// Command notifier drains push_deliveries and sends each row to its device's
// push service.
//
// It is a separate binary rather than an eighth harvester job for three reasons
// that point the same way. The VAPID private key is mounted into this container
// and nowhere else. An outbound call to a third party is the one thing in this
// stack that can hang for minutes, and here it hangs alone rather than starving
// six unrelated jobs. And most of all, this is the delivery path whose
// *silence* is the failure mode: a process boundary gives `up{job=...}`, an
// isolated restart counter and a per-process log stream for free, whereas
// inside a multiplexed binary "the process is up" says nothing about whether
// notifications are still going out. This stack has already lost a delivery
// path for four weeks without noticing (PM-2026-045).
//
// It serves no API. The only listener is the ops listener shared by all the
// binaries — /health for the compose probe, /metrics for Prometheus.
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

const serviceName = "alt-notifier"

func main() {
	if bootstrap.IsHealthcheckInvocation(os.Args) {
		runHealthcheck()
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rt := bootstrap.MustBoot(ctx, bootstrap.Options{ServiceName: serviceName})
	defer rt.Shutdown(ctx)

	cfg := rt.Cfg
	log := rt.Log

	if err := config.ValidateNotifierConfig(cfg); err != nil {
		log.ErrorContext(ctx, "notifier config invalid", "error", err)
		os.Exit(1)
	}
	opsAddr, err := config.LoadOpsListenAddr()
	if err != nil {
		log.ErrorContext(ctx, "ops listener config invalid", "error", err)
		os.Exit(1)
	}
	if err := bootstrap.StartEnrollment(ctx, rt, serviceName); err != nil {
		log.ErrorContext(ctx, "pki enrollment failed", "error", err)
		os.Exit(1)
	}

	container := di.NewNotifierComponents(ctx, cfg)

	scheduler := job.NewJobScheduler()
	job.RegisterNotifierJobs(scheduler, container)
	log.InfoContext(ctx, "notifier.jobs.wiring", "jobs", scheduler.JobNames())
	scheduler.Start(ctx)

	bootstrap.LogOpsWiring(ctx, log, opsAddr, rt.MetricsHandler != nil)
	opsSrv := bootstrap.NewOpsServer(opsAddr, bootstrap.NewOpsHandler(serviceName, rt.MetricsHandler), cfg)

	sup := bootstrap.NewSupervisor(log)
	sup.AddServer("ops", opsSrv.ListenAndServe, opsSrv.Shutdown)
	// Registered as a task so GracefulShutdown drains an in-flight pass before
	// the listener closes; the root context is cancelled first, which is what
	// lets a pass return instead of waiting out its interval.
	sup.AddTask("dispatcher", scheduler.Shutdown)
	sup.Start(ctx)

	outcome := sup.Wait(ctx)
	bootstrap.LogMemStats(ctx, log)
	cancel()
	sup.GracefulShutdown(ctx, cfg.Server.WriteTimeout)

	log.Info("notifier stopped", "reason", outcome.Reason, "signal", outcome.Signal, "server", outcome.Server)
}

// runHealthcheck self-probes the ops listener and exits.
//
// It deliberately does not probe the data plane or a push service. A dispatcher
// that cannot reach alt-data-hub is a real outage, but restarting the container
// does not fix it, and a health check that fails on someone else's unavailability
// turns one service's incident into two.
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
