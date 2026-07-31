import { sveltekit } from "@sveltejs/kit/vite";
import { playwright } from "@vitest/browser-playwright";
import { defineConfig } from "vitest/config";

// Browser-mode component tests stay opt-in because they need a real Chromium
// that `bun install` does not fetch. Opting out is announced instead of being
// inferred silently: without the warning below, a workflow that forgets
// VITEST_BROWSER collects zero of the src/**/*.svelte.{test,spec}.ts files and
// still reports a green run, which is exactly how they went unrun in CI.
const isBrowserTestEnabled = process.env.VITEST_BROWSER === "true";

if (!isBrowserTestEnabled) {
	console.warn(
		"[vitest] client project DISABLED (VITEST_BROWSER != 'true'): " +
			"src/**/*.svelte.{test,spec}.{ts,tsx} are not being collected. " +
			"Use `bun run test:client` / `bun run test:all` to run them.",
	);
}

// The sandbox's cached Playwright Chromium build predates the revision
// playwright-core's zero-config resolver expects (browsers.json pins a
// newer revision than what's on disk under /opt/pw-browsers). Point the
// provider at the cached binary directly instead of leaving the whole
// browser-mode test project unrunnable; when the env var is unset (e.g. a
// normal dev machine with a fully synced Playwright cache) this falls back
// to Playwright's own resolution, so it is a no-op outside this sandbox.
const cachedChromiumExecutable = process.env.VITEST_CHROMIUM_EXECUTABLE_PATH;

export default defineConfig({
	plugins: [sveltekit()],
	cacheDir: "node_modules/.vite",
	server: {
		// Vite mirrors every browser console call back to the terminal and
		// source-maps each one, on top of the copy vitest already captures
		// and attributes to a test. That duplicate is not free: on
		// FeedDetails.svelte.test.ts, whose component retries a failed fetch
		// without a bound at ~4,000 console.error/sec, one 40-second window
		// produced 259,921 forwarded lines (41 MB) against vitest's own
		// 5,267 for the same events — a 49x amplifier that pegged the node
		// process as well as the renderer.
		//
		// Nothing diagnostic is lost by turning it off: vitest still prints
		// each message as `stderr | <test name>`, which is strictly more
		// useful, and unhandledErrors keeps the reporting that only Vite
		// does. Measured across the whole client suite this cut the log from
		// tens of MB to 70 KB.
		forwardConsole: { unhandledErrors: true, logLevels: [] },
	},
	optimizeDeps: {
		exclude: [
			"bits-ui",
			"@lucide/svelte",
			"@threlte/core",
			"@threlte/extras",
			"@tanstack/svelte-query",
		],
	},
	test: {
		globals: true,
		projects: [
			// Browser tests (enabled via VITEST_BROWSER=true)
			...(isBrowserTestEnabled
				? [
						{
							extends: true,
							test: {
								name: "client",
								browser: {
									enabled: true,
									headless: true,
									provider: playwright({
										launchOptions: {
											...(cachedChromiumExecutable
												? { executablePath: cachedChromiumExecutable }
												: {}),
											// Hard ceiling on the renderer heap. Browser mode
											// runs the test runner inside the page, so a
											// component that loops forever starves vitest's own
											// testTimeout and no JS-level timeout can fire —
											// measured on FeedDetails.svelte.test.ts, the
											// renderer burned 122% CPU while the node process
											// sat at 0%, growing unbounded and wedging the
											// whole file pool for as long as the run lasted.
											// V8 still enforces this limit from outside JS, so
											// the runaway OOMs and the run ends instead of
											// hanging. Healthy specs peak around 200-450 MB of
											// process RSS, so this is ~4x headroom; it is a
											// safety ceiling, not a budget.
											args: ["--js-flags=--max-old-space-size=2048"],
										},
									}),
									instances: [{ browser: "chromium" }],
								},
								include: ["src/**/*.svelte.{test,spec}.{ts,tsx}"],
								exclude: ["src/lib/server/**"],
								setupFiles: ["./vitest-setup-client.ts"],
								// Spelled out rather than left to vitest's
								// browser-mode defaults (15s/30s) so the
								// ceiling on a wedged spec is a stated
								// decision. The slowest legitimate spec here
								// is ~1.5s (hard sleeps plus one matcher
								// retry window), so 15s is pure headroom.
								testTimeout: 15_000,
								hookTimeout: 30_000,
							},
						},
					]
				: []),
			// Server tests (always enabled)
			{
				extends: true,
				test: {
					name: "server",
					environment: "node",
					include: ["src/**/*.{test,spec}.{ts,tsx}"],
					exclude: ["src/**/*.svelte.{test,spec}.{ts,tsx}"],
					coverage: {
						provider: "v8",
						include: ["src/lib/**/*.ts", "src/lib/**/*.svelte"],
						exclude: ["src/lib/gen/**"],
						thresholds: {
							lines: 60,
							branches: 50,
							functions: 55,
						},
					},
				},
			},
		],
	},
	resolve: {
		conditions: ["browser"],
	},
});
