import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
	testDir: "./tests/e2e",
	fullyParallel: true,
	forbidOnly: !!process.env.CI,
	retries: process.env.CI ? 2 : 0,
	workers: process.env.CI ? 1 : undefined,
	reporter: "html",
	globalSetup: "./tests/e2e/global-setup",
	globalTeardown: "./tests/e2e/global-teardown",
	use: {
		trace: "on-first-retry",
		baseURL: "http://127.0.0.1:4174/sv/",
		storageState: "tests/e2e/.auth/storage.json",
	},
	projects: [
		{
			name: "chromium",
			use: { ...devices["Desktop Chrome"] },
		},
		{
			name: "firefox",
			use: { ...devices["Desktop Firefox"] },
		},
		{
			name: "webkit",
			use: { ...devices["Desktop Safari"] },
		},
		{
			name: "Mobile Chrome",
			use: { ...devices["Pixel 5"] },
		},
		{
			name: "Mobile Safari",
			use: { ...devices["iPhone 12"] },
		},
	],
	webServer: {
		command: "bun run build && node build",
		url: "http://127.0.0.1:4174/sv/health",
		reuseExistingServer: !process.env.CI,
		stdout: "pipe",
		stderr: "pipe",
		timeout: 120 * 1000,
		env: {
			PORT: "4174",
			ORIGIN: "http://127.0.0.1:4174",
			E2E_TEST_MODE: "true",
			KRATOS_INTERNAL_URL: "http://127.0.0.1:4001",
			AUTH_HUB_INTERNAL_URL: "http://127.0.0.1:4002",
			BACKEND_BASE_URL: "http://127.0.0.1:4003",
		},
	},
});
