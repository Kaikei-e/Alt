import { defineConfig } from "@playwright/test";
import type { ReporterDescription } from "@playwright/test";

/**
 * alt-backend API E2E — Playwright configuration.
 *
 * This suite drives HTTP only: REST on :9000, Connect-RPC on :9101, the
 * loopback operator listener on :9102, and the shared ops listener on :9110.
 * No spec ever touches `page`, `browser` or `context`, so Playwright never
 * launches a browser and the runner needs no browser binaries at all — which
 * is why `run.sh` and CI install with PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1 into
 * a plain node image instead of pulling the ~1.5GB Playwright image.
 *
 * Sharding
 * --------
 * `fullyParallel: true` makes Playwright distribute *individual tests* rather
 * than whole files across shards and workers, which is what keeps a shard
 * split balanced when one file (topology) holds 30 assertions and another
 * holds 2.
 *
 * Reporters
 * ---------
 * Under `PW_BLOB=1` (what the sharded CI matrix sets) each shard writes a
 * blob; a follow-up job merges them into one HTML report + one JUnit file.
 * Otherwise the run writes HTML + JUnit directly, matching what the Hurl
 * runner used to leave under e2e/reports/.
 */

const isCI = !!process.env.CI;
const useBlob = process.env.PW_BLOB === "1";
const reportDir = process.env.PW_REPORT_DIR ?? "./playwright-report";

/**
 * API tests are I/O-bound: a worker spends nearly all of its time waiting on
 * the backend, so the useful worker count is well above the core count. The
 * ceiling is not CPU but alt-backend's DB pool (DB_MAX_CONNS=10 in the
 * staging slice) — 6 keeps a request per worker comfortably inside it while
 * still cutting wall-clock roughly 6x versus serial.
 */
const workers = process.env.PW_WORKERS
	? Number.parseInt(process.env.PW_WORKERS, 10)
	: 6;

const reporters: ReporterDescription[] = [
	["list"],
	...(useBlob
		? ([["blob", { outputDir: `${reportDir}/blob` }]] as ReporterDescription[])
		: ([
				["html", { outputFolder: `${reportDir}/html`, open: "never" }],
				["junit", { outputFile: `${reportDir}/junit.xml` }],
			] as ReporterDescription[])),
	...(isCI ? ([["github"]] as ReporterDescription[]) : []),
];

export default defineConfig({
	testDir: "./tests",
	outputDir: `${reportDir}/test-results`,

	fullyParallel: true,
	forbidOnly: isCI,
	workers,

	/**
	 * Retries exist for the two genuinely non-deterministic things in this
	 * stack — the deps-stub's cold start and alt-backend's shared circuit
	 * breaker (10 consecutive 5xx opens it for 60s, and several scenarios
	 * legitimately assert 5xx). They do NOT exist to paper over an ordering
	 * bug: every spec is self-seeding and shares no mutable state with any
	 * other, so a test that only passes on retry is a real finding.
	 */
	retries: isCI ? 2 : 0,

	timeout: 30_000,
	expect: { timeout: 10_000 },

	globalSetup: "./setup/global-setup.ts",

	reporter: reporters,

	use: {
		// Diagnostics: every request/response of a failing test lands in the
		// trace, which is the API-suite equivalent of Hurl's --report-html.
		trace: "retain-on-failure",
	},

	projects: [
		{
			name: "api",
			testMatch: /tests\/.*\.spec\.ts$/,
		},
	],
});
