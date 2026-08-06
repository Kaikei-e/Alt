import { defineConfig } from "@playwright/test";
import type { PlaywrightTestConfig, ReporterDescription } from "@playwright/test";

/**
 * The one Playwright config every Alt API suite is built from.
 *
 * Twelve hand-written configs drift: one grows a `retries: 3` to quiet a
 * flake, another forgets `forbidOnly`, and the fleet slowly stops meaning the
 * same thing by "green". A factory makes the policy a single artifact and
 * leaves each suite with the two or three facts that are genuinely its own —
 * its name, its worker ceiling, its extra projects.
 *
 * None of these suites drives a browser. Every spec is HTTP-only, so
 * Playwright never launches one and the runner needs no browser binaries at
 * all — which is why run.sh and CI install with
 * PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1 into a plain node image (~150MB) instead
 * of pulling the ~1.7GB Playwright image.
 */

export type ApiSuiteOptions = {
	/**
	 * The service under test, e.g. "auth-hub".
	 *
	 * Becomes `TestConfig.tag` (Playwright 1.57+), which the blob reporter
	 * folds into the blob filename and carries through `merge-reports`. That
	 * is what lets all twelve suites' blobs merge into one HTML report without
	 * colliding, and what labels each test with the service it came from.
	 */
	readonly service: string;

	/**
	 * Worker ceiling for this suite.
	 *
	 * Size this by the **downstream** bottleneck, not by CPU. API tests are
	 * I/O-bound — a worker spends nearly all its time waiting — so the useful
	 * count is well above the core count, and the real ceiling is whatever the
	 * service will run out of first: its DB pool, its rate limiter, an upstream
	 * quota. A service with DB_MAX_CONNS=10 comfortably takes 6.
	 *
	 * Playwright's own CI guidance ("set workers to 1") is browser-memory
	 * advice and does not apply here; following it would make this fleet
	 * dramatically slower than the Hurl suites it replaces, for no benefit.
	 */
	readonly workers: number;

	/** Per-test budget. Raise only for a suite with genuinely slow endpoints. */
	readonly timeout?: number;

	/** Extra projects — e.g. a `workers: 1` project for rate-limit specs. */
	readonly projects?: PlaywrightTestConfig["projects"];

	/** Path to a globalSetup module, relative to the suite directory. */
	readonly globalSetup?: string;

	/** `testDir` override. Defaults to `./tests`. */
	readonly testDir?: string;
};

/**
 * Retry intervals for `toPass`, tuned for Alt rather than for a browser.
 *
 * Playwright's default (`[100, 250, 500, 1000]`) is sized for UI settling. The
 * things these suites wait on — a Redis Streams consumer, an event projector,
 * a Meilisearch index — run on a 1–5s cadence, so 100ms probes only add load
 * to the service under test without converging any sooner.
 */
const TO_PASS_INTERVALS = [500, 1_000, 2_000] as const;

export function defineApiSuite(options: ApiSuiteOptions): PlaywrightTestConfig {
	const isCI = !!process.env.CI;
	const useBlob = process.env.PW_BLOB === "1";
	const reportDir = process.env.PW_REPORT_DIR ?? "./playwright-report";

	const workers = process.env.PW_WORKERS
		? Number.parseInt(process.env.PW_WORKERS, 10)
		: options.workers;

	/**
	 * Under `PW_BLOB=1` each shard writes a blob and a follow-up job merges
	 * them into one HTML + JUnit report.
	 *
	 * The `github` reporter is deliberately **absent** here even on CI.
	 * Playwright's own docs advise against it under a matrix strategy — with a
	 * dozen services in the matrix, one shared-fixture breakage emits dozens of
	 * duplicate inline annotations on the same lines and buries the file view.
	 * The merge job emits them exactly once instead, via
	 * `merge-reports --reporter=html,github`.
	 */
	const reporters: ReporterDescription[] = useBlob
		? [["blob", { outputDir: `${reportDir}/blob` }]]
		: [
				[isCI ? "dot" : "list"],
				["html", { outputFolder: `${reportDir}/html`, open: "never" }],
				["junit", { outputFile: `${reportDir}/junit.xml` }],
			];

	return defineConfig({
		testDir: options.testDir ?? "./tests",
		outputDir: `${reportDir}/test-results`,

		/**
		 * Distributes *individual tests* rather than whole files across shards
		 * and workers, which is what keeps a split balanced when one file holds
		 * thirty assertions and another holds two. It is also a standing
		 * constraint on how specs are written: every test in these suites seeds
		 * what it reads under a name no other test uses, so no test may assume
		 * another ran first.
		 */
		fullyParallel: true,
		forbidOnly: isCI,
		workers,

		/**
		 * Retries exist for genuinely non-deterministic infrastructure — a
		 * container's cold start, a circuit breaker several scenarios
		 * legitimately trip. They do NOT exist to paper over an ordering bug:
		 * every spec is self-seeding and shares no mutable state, so a test that
		 * only passes on retry is a real finding.
		 */
		retries: isCI ? 2 : 0,

		/**
		 * Retry at the *end* of the run, one at a time, in a single worker
		 * (Playwright 1.62+).
		 *
		 * The failures these suites retry are cross-test interference —
		 * a shared circuit breaker, a shared rate-limit bucket, a dependency
		 * still warming. Retrying immediately, while five other workers are
		 * still hammering the same shared thing, is the worst possible moment.
		 * Isolating the retry turns "retry to survive interference" into "retry
		 * to confirm a real failure", which is the only semantics worth having.
		 */
		retryStrategy: "isolated",

		/**
		 * A test that passes only on retry exits 0 by default, which is exactly
		 * how `retries: 2` comes to hide real instability. Turning this on for
		 * every PR would make each infrastructure hiccup a red PR, so it is
		 * gated behind PW_STRICT_FLAKE and set on the nightly run, where a flake
		 * is something to investigate rather than something to re-run.
		 */
		failOnFlakyTests: process.env.PW_STRICT_FLAKE === "1",

		timeout: options.timeout ?? 30_000,

		expect: {
			timeout: 10_000,
			/**
			 * `toPass` defaults to **timeout 0 — unlimited** and ignores
			 * `expect.timeout`. An un-optioned `toPass` therefore polls until the
			 * whole test times out and reports a generic timeout instead of the
			 * assertion that failed. This backstop makes that impossible.
			 */
			toPass: { timeout: 20_000, intervals: [...TO_PASS_INTERVALS] },
		},

		...(options.globalSetup === undefined ? {} : { globalSetup: options.globalSetup }),

		reporter: reporters,

		/**
		 * Run-level tag (Playwright 1.57+). The blob reporter folds it into the
		 * blob filename, so twelve services' blobs merge without collision and
		 * every test in the merged report says which service it came from.
		 */
		tag: `@${options.service}`,

		use: {
			// Every request/response of a failing test lands in the trace, which
			// is this fleet's replacement for Hurl's --report-html.
			trace: "retain-on-failure",
		},

		projects: options.projects ?? [
			{
				name: "api",
				testMatch: /.*\.spec\.ts$/,
			},
		],
	});
}
