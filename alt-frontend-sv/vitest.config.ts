import { sveltekit } from "@sveltejs/kit/vite";
import { playwright } from "@vitest/browser-playwright";
import { defineConfig } from "vitest/config";

const isBrowserTestEnabled = process.env.VITEST_BROWSER === "true";

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
									provider: playwright(
										cachedChromiumExecutable
											? {
													launchOptions: {
														executablePath: cachedChromiumExecutable,
													},
												}
											: undefined,
									),
									instances: [{ browser: "chromium" }],
								},
								include: ["src/**/*.svelte.{test,spec}.{ts,tsx}"],
								exclude: ["src/lib/server/**"],
								setupFiles: ["./vitest-setup-client.ts"],
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
