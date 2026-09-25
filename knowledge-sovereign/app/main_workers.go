package main

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"knowledge-sovereign/config"
	"knowledge-sovereign/driver/sovereign_db"
	"knowledge-sovereign/usecase/knowledge_home_projector"
	"knowledge-sovereign/usecase/knowledge_trail_projector"
	"knowledge-sovereign/usecase/partition_maintainer"
	"knowledge-sovereign/usecase/projection_health"
	"knowledge-sovereign/usecase/trail_planner"
)

// partitionRunner is the ensure-step surface main wires. Kept as an interface
// so the startup wiring itself is exercisable without a database.
type partitionRunner interface {
	RunOnce(ctx context.Context) error
}

// startPartitionMaintainer runs the partition ensure-step once at startup and
// then on a slow tick. The startup run matters: the tick is hours long, and a
// replica booting into a month with no partition would otherwise keep writing
// into the DEFAULT partition until the first tick.
//
// It returns immediately: the startup ensure runs inside the goroutine, ahead
// of the ticker loop, never on the caller's. This is the only unbounded DB call
// on main()'s startup path, and it issues DDL that takes ACCESS EXCLUSIVE on
// the hot append table. Run inline it would sequence both projectors, the
// branch planner, the projection_health exporter and signal.Notify behind a
// lock wait, leaving a service that answers /health 200 on both ports with no
// projection running and no graceful shutdown — the silent-degradation shape
// rule 8 exists to prevent (ADR-000928 / PM-2026-045).
//
// A failure is logged, never fatal — the default partition still accepts the
// writes, so a crashloop would cost more than the degraded pruning does.
func startPartitionMaintainer(ctx context.Context, wg *sync.WaitGroup, runner partitionRunner, interval time.Duration) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := runner.RunOnce(ctx); err != nil {
			slog.Error("partition_maintainer startup ensure failed", "error", err)
		}

		tick := time.NewTicker(interval)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				if err := runner.RunOnce(ctx); err != nil {
					slog.Error("partition_maintainer batch failed", "error", err)
				}
			}
		}
	}()
}

func startWorkers(ctx context.Context, wg *sync.WaitGroup, repo *sovereign_db.Repository, cfg *config.Config) {
	// Monthly partition maintenance for the append-only event tables. The
	// migrations created a fixed six months each and nothing renewed them, so
	// everything past 2026-05-01 fell into the DEFAULT partition; this creates
	// the current month plus a lookahead, idempotently and under an advisory
	// lock so every replica can run it. Rule 8: log the wiring state loudly —
	// a generator with no caller is precisely how this went unnoticed.
	partitionMaintainer := partition_maintainer.New(repo, slog.Default(), partition_maintainer.Config{})
	slog.Info("partition.maintainer.wiring", "enabled", true, "repository_wired", repo != nil)
	startPartitionMaintainer(ctx, wg, partitionMaintainer, partition_maintainer.DefaultTickInterval)

	// Knowledge Trail spine projector. Folds the append-only event log into
	// knowledge_trail_footprints in-process. Reproject-safe and idempotent, so a
	// short tick over a quiet log is cheap.
	trailProjector := knowledge_trail_projector.NewProjector(repo, slog.Default(),
		knowledge_trail_projector.Config{
			BatchSize:         cfg.TrailProjectorBatchSize,
			MaxBatchesPerTick: cfg.TrailProjectorMaxBatches,
		})
	trailTick := time.NewTicker(cfg.ProjectorTickInterval)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer trailTick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-trailTick.C:
				if err := trailProjector.RunBatch(ctx); err != nil {
					slog.Error("knowledge_trail_projector batch failed", "error", err)
				}
			}
		}
	}()

	// Knowledge Home projector. Folds the same append-only event log into the
	// Knowledge Home read models (knowledge_home_items, today_digest_view,
	// recall_candidate_view) in-process. Reproject-safe and idempotent, same
	// shape as knowledge_trail_projector above; shares its tick interval since
	// both drain the same event log on the same cadence. Rule 8: surface the
	// wiring state loudly at startup so a missing projector is not
	// indistinguishable from an intentionally-disabled one (PM-2026-045 /
	// ADR-000928).
	homeProjector := knowledge_home_projector.NewProjector(repo, slog.Default(),
		knowledge_home_projector.Config{
			BatchSize:         cfg.HomeProjectorBatchSize,
			MaxBatchesPerTick: cfg.HomeProjectorMaxBatches,
		})
	slog.Info("home.projector.wiring", "enabled", true, "repository_wired", repo != nil)
	homeTick := time.NewTicker(cfg.ProjectorTickInterval)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer homeTick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-homeTick.C:
				if err := homeProjector.RunBatch(ctx); err != nil {
					slog.Error("knowledge_home_projector batch failed", "error", err)
				}
			}
		}
	}()

	// Knowledge Trail branch producer (trail_planner). Rule 8: surface the wiring
	// state loudly at startup so a missing producer is visible immediately, not
	// as a silent absence of branches weeks later (PM-2026-045 / ADR-000928).
	// NewPlanner always returns non-nil; the real wiring signal is whether the
	// repository dependency was supplied.
	branchPlanner := trail_planner.NewPlanner(repo, slog.Default(), trail_planner.Config{
		MaxBranchesPerUser: cfg.TrailMaxBranchesPerUser,
		Clock:              time.Now,
	})
	slog.Info("trail.branch_producer.wiring", "enabled", true, "repository_wired", repo != nil)
	branchTick := time.NewTicker(cfg.BranchPlannerTickInterval)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer branchTick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-branchTick.C:
				if err := branchPlanner.RunBatch(ctx); err != nil {
					slog.Error("trail_planner batch failed", "error", err)
				}
			}
		}
	}()

	// Producer-liveness gauges sampled on a slow tick.
	healthExporter := projection_health.New(repo, slog.Default())
	healthTick := time.NewTicker(cfg.ProjectionHealthTickInterval)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer healthTick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-healthTick.C:
				if err := healthExporter.RunOnce(ctx); err != nil {
					slog.Error("projection_health exporter failed", "error", err)
				}
			}
		}
	}()
}
