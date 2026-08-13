import { defineConfig, devices } from "@playwright/test";

/**
 * Project layout
 * --------------
 * Every spec belongs to exactly one suite directory under `tests/e2e/`, and
 * every suite directory is claimed by the projects below via a `testMatch` that
 * is anchored on that directory. Nothing matches "whatever is left over":
 * a previous generation of this config carried four catch-all projects
 * (`chromium` / `webkit` / `Mobile Chrome` / `Mobile Safari`) whose negative
 * lookaheads swept up the a11y, visual and integration suites and ran each of
 * them a further four times — 842 test invocations for 355 real ones, with the
 * mobile a11y audit executing in a desktop viewport and the integration suite
 * pointed at the preview server instead of `ALT_RUNTIME_URL`.
 *
 * Adding a suite means adding a project. That is the point.
 */

const isCI = !!process.env.CI;

/**
 * Preview-server port. Defaults to 4174 so CI and every existing invocation
 * behave exactly as before; override with ALT_E2E_PORT when 4174 is already
 * claimed on a developer machine by something unrelated to this repo. The
 * value has to reach four places at once — Playwright's baseURL, the
 * readiness probe, and the server's own PORT/ORIGIN — because adapter-node
 * checks ORIGIN on every form POST, so a mismatch fails as a CSRF rejection
 * rather than as a connection error.
 */
const previewPort = process.env.ALT_E2E_PORT || "4174";
const previewOrigin = `http://127.0.0.1:${previewPort}`;

export default defineConfig({
	testDir: "./tests/e2e",
	fullyParallel: true,
	forbidOnly: isCI,
	retries: isCI ? 2 : 0,
	workers: isCI ? 2 : undefined,

	// Retries run at the end in a single worker so a retried test is not
	// competing for the preview server with the rest of the shard — the flake we
	// are trying to confirm should not be re-created by the retry itself.
	retryStrategy: "isolated",

	// CI-optimized timeouts
	timeout: isCI ? 60_000 : 30_000,
	expect: {
		timeout: isCI ? 15_000 : 5_000,
		toHaveScreenshot: {
			maxDiffPixelRatio: 0.01,
			// Freeze CSS animations and the text caret so a snapshot cannot
			// disagree with itself between runs.
			animations: "disabled",
			caret: "hide",
		},
	},

	// Keep the OS out of the snapshot key: the visual suite only ever runs on
	// Linux (in the Playwright container locally and in CI), so the default
	// `{platform}` suffix only produced churn.
	snapshotPathTemplate:
		"{testDir}/{testFileDir}/__screenshots__/{testFileName}/{projectName}/{arg}{ext}",

	// Enhanced reporters
	reporter: [
		["list"],
		["html", { open: "never" }],
		...(isCI
			? [
					["github"] as const,
					["json", { outputFile: "test-results/results.json" }] as const,
				]
			: []),
	],

	globalSetup: "./tests/e2e/global-setup",
	globalTeardown: "./tests/e2e/global-teardown",

	use: {
		// CI-optimized action timeouts
		actionTimeout: isCI ? 15_000 : 10_000,
		navigationTimeout: isCI ? 30_000 : 15_000,

		// Enhanced tracing for debugging
		trace: "retain-on-failure",
		screenshot: "only-on-failure",
		video: "retain-on-failure",

		testIdAttribute: "data-testid",

		baseURL: `${previewOrigin}/`,
		storageState: "tests/e2e/.auth/storage.json",
	},

	projects: [
		// Auth flows run without the pre-authenticated storage state: the whole
		// point is to observe the app from a signed-out browser.
		{
			name: "auth",
			testMatch: /tests\/e2e\/auth\/.*\.spec\.ts$/,
			use: {
				...devices["Desktop Chrome"],
				storageState: { cookies: [], origins: [] },
			},
		},

		// Desktop feature suites.
		{
			name: "desktop-chromium",
			testMatch: /tests\/e2e\/desktop\/.*\.spec\.ts$/,
			use: { ...devices["Desktop Chrome"] },
		},
		{
			name: "desktop-webkit",
			testMatch: /tests\/e2e\/desktop\/.*\.spec\.ts$/,
			use: { ...devices["Desktop Safari"] },
		},

		// Mobile feature suites.
		{
			name: "mobile-chrome",
			testMatch: /tests\/e2e\/mobile\/.*\.spec\.ts$/,
			use: { ...devices["Pixel 5"] },
		},
		{
			name: "mobile-safari",
			testMatch: /tests\/e2e\/mobile\/.*\.spec\.ts$/,
			use: { ...devices["iPhone 12"] },
		},

		// Accessibility suites, each in the form factor it is written for. These
		// used to have no project of their own and were only reachable through
		// the catch-all projects, which meant the mobile audit ran at 1280x720 —
		// and CI, which selects projects by name, never ran them at all.
		{
			name: "a11y-desktop",
			testMatch: /tests\/e2e\/a11y\/desktop\..*\.spec\.ts$/,
			use: { ...devices["Desktop Chrome"] },
		},
		{
			name: "a11y-mobile",
			testMatch: /tests\/e2e\/a11y\/mobile\..*\.spec\.ts$/,
			use: { ...devices["Pixel 5"] },
		},

		// Visual regression. Individual specs override the viewport via
		// `test.use()` when they target a mobile layout.
		{
			name: "visual-regression",
			testMatch: /tests\/e2e\/visual\/.*\.spec\.ts$/,
			use: {
				...devices["Desktop Chrome"],
				viewport: { width: 1280, height: 720 },
			},
		},

		// Integration E2E tests (against a real backend on the runtime machine).
		// globalSetup/globalTeardown per-project is supported at runtime but not
		// yet in Playwright's type defs.
		{
			name: "integration",
			testMatch: /tests\/e2e\/integration\/.*\.spec\.ts$/,
			use: {
				...devices["Desktop Chrome"],
				baseURL: process.env.ALT_RUNTIME_URL || "http://localhost:4173/",
			},
			...({
				globalSetup: "./tests/e2e/integration/global-setup",
				globalTeardown: "./tests/e2e/integration/global-teardown",
			} as Record<string, string>),
		},
	],

	// The integration project targets an already-running stack (ALT_RUNTIME_URL /
	// :4173) and does not need the local preview server; PW_NO_WEBSERVER=1 skips
	// it when the preview port is unavailable (e.g. taken by an unrelated
	// container). Prefer ALT_E2E_PORT over PW_NO_WEBSERVER in that case: skipping
	// the server leaves baseURL pointing at whatever else is on the port, so the
	// suite runs green or red against a foreign app instead of failing loudly.
	webServer: process.env.PW_NO_WEBSERVER
		? undefined
		: {
				command: "bun run build && node build",
				url: `${previewOrigin}/health`,
				reuseExistingServer: !isCI,
				stdout: "pipe",
				stderr: "pipe",
				timeout: 120 * 1000,
				env: {
					PORT: previewPort,
					ORIGIN: previewOrigin,
					E2E_TEST_MODE: "true",
					KRATOS_INTERNAL_URL: "http://127.0.0.1:4001",
					KRATOS_PUBLIC_URL: "http://127.0.0.1:4001",
					AUTH_HUB_INTERNAL_URL: "http://127.0.0.1:4002",
					BACKEND_BASE_URL: "http://127.0.0.1:4003",
					BACKEND_CONNECT_URL: "http://127.0.0.1:4003",
					RECAP_WORKER_BASE_URL: "http://127.0.0.1:4003",
					// No knowledge-sovereign runs here and the admin panel is out of
					// scope for this suite, so opt out of its Bearer explicitly.
					SOVEREIGN_ADMIN_AUTH: "disabled",
				},
			},
});
