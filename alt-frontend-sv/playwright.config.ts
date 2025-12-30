import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
	testDir: "./tests/e2e",
	fullyParallel: true,
	forbidOnly: !!process.env.CI,
	retries: process.env.CI ? 2 : 0,
	workers: process.env.CI ? 1 : undefined,

	// Enhanced reporters
	reporter: [
		["list"],
		["html", { open: "never" }],
		...(process.env.CI ? [["json" as const, { outputFile: "test-results/results.json" }]] : []),
	],

	globalSetup: "./tests/e2e/global-setup",
	globalTeardown: "./tests/e2e/global-teardown",

	use: {
		// Enhanced tracing for debugging
		trace: "retain-on-failure",
		screenshot: "only-on-failure",
		video: "retain-on-failure",

		baseURL: "http://127.0.0.1:4174/sv/",
		storageState: "tests/e2e/.auth/storage.json",
	},

	projects: [
		// Auth tests (no pre-authenticated storage)
		{
			name: "auth",
			testMatch: /auth\/.*\.spec\.ts/,
			use: {
				...devices["Desktop Chrome"],
				storageState: { cookies: [], origins: [] },
			},
		},

		// Desktop tests - Chromium
		{
			name: "desktop-chromium",
			testMatch: /desktop\/.*\.spec\.ts/,
			use: { ...devices["Desktop Chrome"] },
		},

		// Desktop tests - WebKit (optional)
		{
			name: "desktop-webkit",
			testMatch: /desktop\/.*\.spec\.ts/,
			use: { ...devices["Desktop Safari"] },
		},

		// Mobile tests - Chrome
		{
			name: "mobile-chrome",
			testMatch: /mobile\/.*\.spec\.ts/,
			use: { ...devices["Pixel 5"] },
		},

		// Mobile tests - Safari
		{
			name: "mobile-safari",
			testMatch: /mobile\/.*\.spec\.ts/,
			use: { ...devices["iPhone 12"] },
		},

		// Legacy projects for backward compatibility
		{
			name: "chromium",
			testMatch: /(?<!desktop\/)(?<!mobile\/)(?<!auth\/).*\.spec\.ts$/,
			testIgnore: /(desktop|mobile|auth)\/.*\.spec\.ts/,
			use: { ...devices["Desktop Chrome"] },
		},
		{
			name: "webkit",
			testMatch: /(?<!desktop\/)(?<!mobile\/)(?<!auth\/).*\.spec\.ts$/,
			testIgnore: /(desktop|mobile|auth)\/.*\.spec\.ts/,
			use: { ...devices["Desktop Safari"] },
		},
		{
			name: "Mobile Chrome",
			testMatch: /(?<!desktop\/)(?<!mobile\/)(?<!auth\/).*\.spec\.ts$/,
			testIgnore: /(desktop|mobile|auth)\/.*\.spec\.ts/,
			use: { ...devices["Pixel 5"] },
		},
		{
			name: "Mobile Safari",
			testMatch: /(?<!desktop\/)(?<!mobile\/)(?<!auth\/).*\.spec\.ts$/,
			testIgnore: /(desktop|mobile|auth)\/.*\.spec\.ts/,
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
