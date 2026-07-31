package job

import (
	"log/slog"
	"time"

	"alt/config"
	"alt/di"
)

// RegisterHarvesterJobs registers cmd/harvester's background jobs.
//
// Two jobs from the single-binary registry are handled differently here, both
// for the same reason: a scheduled job that runs and achieves nothing is worse
// than one that is absent, because its success log reads identically to a
// working one (CLAUDE.md rule 8).
//
//   - tag-cloud-cache-warmer is not registered at all. See
//     logTagCloudWarmerDisabled.
//   - The two image jobs are registered only when the image proxy is wired.
func RegisterHarvesterJobs(scheduler *JobScheduler, container *di.HarvesterComponents, cfg *config.Config) {
	scheduler.Add(Job{
		Name:     "hourly-feed-collector",
		Interval: 1 * time.Hour,
		Timeout:  30 * time.Minute,
		Fn:       CollectFeedsJob(container.AltDBRepository),
	})
	scheduler.Add(Job{
		Name:     "daily-scraping-policy",
		Interval: 24 * time.Hour,
		Timeout:  1 * time.Hour,
		Fn:       ScrapingPolicyJob(container.ScrapingDomainUsecase),
	})
	scheduler.Add(Job{
		Name:     "outbox-worker",
		Interval: 5 * time.Second,
		// A batch of up to 10 ARTICLE_UPSERT events can each take 10-30s on the
		// local CPU/GPU embedder for heavy articles (500+KB, 100+ chunks), so
		// 30s was not enough headroom for a full batch. Runs never overlap:
		// JobScheduler.runJob calls executeJob synchronously in a single
		// goroutine per job, so the next tick only starts after this run
		// returns (see scheduler.go), regardless of Interval vs Timeout.
		Timeout: 5 * time.Minute,
		Fn:      OutboxWorkerJob(container.AltDBRepository, container.RagIntegration, container.SovereignClient),
	})
	scheduler.Add(Job{
		Name:     "og-image-retention",
		Interval: 6 * time.Hour,
		Timeout:  10 * time.Minute,
		Fn:       OgImageRetentionJob(container.AltDBRepository),
	})
	scheduler.Add(Job{
		Name:     "outbox-prune",
		Interval: 24 * time.Hour,
		Timeout:  5 * time.Minute,
		Fn:       OutboxPruneJob(container.AltDBRepository),
	})

	registerImageJobs(scheduler, container)
	logTagCloudWarmerDisabled()
}

// registerImageJobs adds the two jobs that drive the OGP image pipeline, or
// states loudly that it is not wired and adds neither.
//
// Both jobs previously fell back to a closure that logged
// "skipped: dependencies_disabled" once per tick when ImageProxyUsecase was
// nil. That is a defensible shape for the backend, where image proxying is one
// feature among dozens; it is not for the harvester, where these two jobs are
// a quarter of the process's workload. Either the pipeline is wired and the
// jobs run, or IMAGE_PROXY_ENABLED=false is explicit and they do not exist.
func registerImageJobs(scheduler *JobScheduler, container *di.HarvesterComponents) {
	if container.ImageProxyUsecase == nil {
		slog.Warn("harvester.image_jobs_disabled",
			"jobs", "ogp-image-warmer,og-image-backfill",
			"reason", "IMAGE_PROXY_ENABLED=false; the jobs are not registered rather than ticking a no-op")
		return
	}

	slog.Info("harvester.image_jobs_enabled", "jobs", "ogp-image-warmer,og-image-backfill")
	scheduler.Add(Job{
		Name:     "ogp-image-warmer",
		Interval: 1 * time.Hour,
		Timeout:  20 * time.Minute,
		Fn:       OgpImageWarmerJob(container.AltDBRepository, container.ImageProxyUsecase),
	})
	scheduler.Add(Job{
		Name:     "og-image-backfill",
		Interval: 30 * time.Minute,
		Timeout:  20 * time.Minute,
		Fn:       OgImageBackfillJob(container.AltDBRepository, container.FetchArticleGateway, container.ImageProxyUsecase),
	})
}

// logTagCloudWarmerDisabled records why the harvester does not run
// tag-cloud-cache-warmer.
//
// FetchTagCloudUsecase caches its result in process memory for 30 minutes.
// While alt-backend was one process, warming that cache on a 24-minute tick
// kept hot the very cache the REST and Connect handlers read from. With the
// scheduler in its own process the warmer only refreshes the harvester's own
// copy — the backend's and the data-hub's are separate objects in separate
// processes and never observe it. Keeping the job would mean logs that say
// "warmed 300 tags" every 24 minutes while every reader still takes the cold
// path: running, green, and worth nothing.
//
// The fetch-side lazy warm inside FetchTagCloudUsecase is unchanged, so the
// first reader after a TTL expiry still pays for the refresh exactly as it did
// before. Moving the cache into a shared store is the real fix and is out of
// scope for the split.
func logTagCloudWarmerDisabled() {
	slog.Warn("harvester.tag_cloud_warmer_disabled",
		"job", "tag-cloud-cache-warmer",
		"reason", "the tag cloud cache is process-local; warming it from the harvester cannot reach the backend or data-hub readers",
		"fallback", "lazy refresh on the first read after the 30m TTL expires")
}
