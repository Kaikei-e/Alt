package job

import (
	"alt/config"
	"alt/di"
	"time"
)

// RegisterAllJobs registers all background jobs with the scheduler.
// This centralizes job configuration that was previously scattered in main.go.
func RegisterAllJobs(scheduler *JobScheduler, container *di.ApplicationComponents, cfg *config.Config) {
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
		Timeout:  30 * time.Second,
		Fn:       OutboxWorkerJob(container.AltDBRepository, container.RagIntegration, container.SovereignClient),
	})
	scheduler.Add(Job{
		Name:     "ogp-image-warmer",
		Interval: 1 * time.Hour,
		Timeout:  20 * time.Minute,
		Fn:       OgpImageWarmerJob(container.AltDBRepository, container.ImageProxyUsecase),
	})
	scheduler.Add(Job{
		Name:     "tag-cloud-cache-warmer",
		Interval: 24 * time.Minute,
		Timeout:  2 * time.Minute,
		Fn:       TagCloudCacheWarmerJob(container.FetchTagCloudUsecase),
	})
	scheduler.Add(Job{
		Name:     "knowledge-backfill",
		Interval: 15 * time.Second,
		Timeout:  25 * time.Second,
		Fn: KnowledgeBackfillJob(
			container.SovereignClient,
			container.SovereignClient,
			container.SovereignClient,
			container.KnowledgeBackfillArticlesGateway,
			container.SovereignClient,
		),
	})
	scheduler.Add(Job{
		Name:     "knowledge-reproject",
		Interval: 30 * time.Second,
		Timeout:  25 * time.Second,
		Fn: KnowledgeReprojectJob(
			container.SovereignClient,
			container.SovereignClient,
			container.SovereignClient,
			container.SovereignClient,
			container.SovereignClient,
			container.SovereignClient,
			container.SovereignClient,
			container.SovereignClient,
			container.SummaryVersionGateway,
			container.TagSetVersionGateway,
		),
	})
	scheduler.Add(Job{
		Name:     "recall-projector",
		Interval: 60 * time.Second,
		Timeout:  30 * time.Second,
		Fn: RecallProjectorJob(
			container.SovereignClient,
			container.SovereignClient,
			container.SovereignClient,
			container.KnowledgeHomeMetrics,
		),
	})
	scheduler.Add(Job{
		Name:     "digest-availability-reconcile",
		Interval: 5 * time.Minute,
		Timeout:  30 * time.Second,
		Fn: DigestAvailabilityReconcileJob(
			container.SovereignClient,
			container.RecapUsecase,
			container.SovereignClient,
		),
	})
}
