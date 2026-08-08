package job

import (
	"testing"

	"alt/config"
	"alt/di"
	"alt/orchestrator/usecase/image_proxy_usecase"
)

func harvesterComponents(imageProxy *image_proxy_usecase.ImageProxyUsecase) *di.HarvesterComponents {
	return &di.HarvesterComponents{
		Config:            &config.Config{},
		ImageProxyUsecase: imageProxy,
	}
}

func registeredNames(t *testing.T, c *di.HarvesterComponents) map[string]bool {
	t.Helper()
	s := NewJobScheduler()
	RegisterHarvesterJobs(s, c, c.Config)
	names := make(map[string]bool)
	for _, n := range s.JobNames() {
		names[n] = true
	}
	return names
}

// tag-cloud-cache-warmer refreshes an in-process cache owned by
// FetchTagCloudUsecase. Once the scheduler moved into its own process, the
// warmer only ever warmed the harvester's own copy — the backend and data-hub
// caches it exists to keep hot are in different processes and never see it.
// Registering it would produce a job that logs success every 24 minutes and
// achieves exactly nothing, so it is not registered at all.
func TestRegisterHarvesterJobs_DoesNotRegisterTheTagCloudWarmer(t *testing.T) {
	names := registeredNames(t, harvesterComponents(nil))

	if names["tag-cloud-cache-warmer"] {
		t.Error("tag-cloud-cache-warmer warms a cache in another process; it must not be registered")
	}
}

// The jobs that do not depend on the image pipeline must always be registered.
func TestRegisterHarvesterJobs_RegistersTheCoreJobs(t *testing.T) {
	names := registeredNames(t, harvesterComponents(nil))

	for _, want := range []string{
		"hourly-feed-collector",
		"daily-scraping-policy",
		"outbox-worker",
		"og-image-retention",
		"outbox-prune",
		"today-entrance-notifier",
	} {
		if !names[want] {
			t.Errorf("%s must be registered", want)
		}
	}
}

// The two image jobs used to be registered unconditionally and fall back to a
// closure that logged "dependencies_disabled" on every tick. In the harvester
// those two jobs are a quarter of the process's reason to exist, so an
// explicit IMAGE_PROXY_ENABLED=false omits them (one startup log) instead of
// ticking a no-op forever.
func TestRegisterHarvesterJobs_ImageJobsFollowTheImageProxyWiring(t *testing.T) {
	tests := []struct {
		name         string
		imageProxy   *image_proxy_usecase.ImageProxyUsecase
		wantRegister bool
	}{
		{name: "image proxy disabled", imageProxy: nil, wantRegister: false},
		{name: "image proxy wired", imageProxy: &image_proxy_usecase.ImageProxyUsecase{}, wantRegister: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			names := registeredNames(t, harvesterComponents(tt.imageProxy))

			for _, job := range []string{"ogp-image-warmer", "og-image-backfill"} {
				if names[job] != tt.wantRegister {
					t.Errorf("%s registered = %v, want %v", job, names[job], tt.wantRegister)
				}
			}
		})
	}
}
