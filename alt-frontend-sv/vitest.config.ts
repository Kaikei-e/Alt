import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { dirname, join } from "node:path";
import { playwright } from "@vitest/browser-playwright";
import type { Plugin } from "vite";
import { defineConfig, mergeConfig } from "vitest/config";
import viteConfig from "./vite.config.ts";

// Browser-mode component tests run native ES modules in Chromium without
// bundling. A barrel `import { X } from "@lucide/svelte"` evaluates
// `dist/lucide-svelte.js`, which statically re-exports all 1,745 `.svelte`
// icons plus 1,745 `.js` alias wrappers (~3,500 HTTP requests per tab). When
// `vi.mock()` is active in a tab, Playwright intercepts `**/*` on that context
// and allocates a `RequestDispatcher` + `RouteDispatcher` per request in a
// shared 10,000-entry bucket (`maxDispatchersForBucket`), causing parallel
// tabs to evict in-flight mock `RouteDispatcher`s with:
// `Error: route.fulfill: The object has been collected to prevent unbounded heap growth`.
// Rewriting barrel value imports to `@lucide/svelte/icons/<name>` keeps real
// icon components while loading only the icons a component actually uses.
function lucideSvelteDirectImports(): Plugin {
	const require = createRequire(import.meta.url);
	const pkgDir = dirname(dirname(require.resolve("@lucide/svelte")));
	const iconMap = new Map<string, string>();
	const exportRe =
		/default\s+as\s+(\w+)\s*\}\s*from\s*['"](?:\.\/|\.\.\/icons\/)([^.'"]+)\.(?:svelte|js)['"]/g;
	for (const rel of [
		"dist/icons/index.js",
		"dist/aliases/aliases.js",
		"dist/aliases/prefixed.js",
		"dist/aliases/suffixed.js",
	]) {
		const content = readFileSync(join(pkgDir, rel), "utf8");
		for (const match of content.matchAll(exportRe)) {
			const exported = match[1];
			const kebab = match[2];
			if (exported && kebab) {
				iconMap.set(exported, kebab);
			}
		}
	}

	const importRe =
		/import\s+(?!type\b)\{([^}]+)\}\s*from\s*["']@lucide\/svelte["']\s*;?/g;

	return {
		name: "vitest-lucide-svelte-direct-imports",
		enforce: "pre",
		transform(code, id) {
			if (id.includes("node_modules") || !code.includes("@lucide/svelte")) {
				return null;
			}
			let changed = false;
			const next = code.replace(importRe, (full, specifiers: string) => {
				const lines: string[] = [];
				const remaining: string[] = [];
				for (const raw of specifiers.split(",")) {
					const spec = raw.trim();
					if (!spec) continue;
					if (spec.startsWith("type ")) {
						remaining.push(spec);
						continue;
					}
					const parts = spec.split(/\s+as\s+/);
					const imported = parts[0]?.trim() ?? "";
					const local = (parts[1] ?? parts[0] ?? "").trim();
					const kebab = iconMap.get(imported);
					if (kebab) {
						lines.push(`import ${local} from "@lucide/svelte/icons/${kebab}";`);
					} else {
						remaining.push(spec);
					}
				}
				if (lines.length === 0) return full;
				changed = true;
				if (remaining.length > 0) {
					lines.push(
						`import { ${remaining.join(", ")} } from "@lucide/svelte";`,
					);
				}
				return lines.join("\n");
			});
			return changed ? { code: next, map: null } : null;
		},
	};
}

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

export default mergeConfig(
	viteConfig,
	defineConfig({
		plugins: [lucideSvelteDirectImports()],
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
			projects: [
				// Browser tests (enabled via VITEST_BROWSER=true)
				...(isBrowserTestEnabled
					? [
							{
								extends: true as const,
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
										instances: [{ browser: "chromium" as const }],
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
					extends: true as const,
					test: {
						name: "server",
						environment: "node",
						include: ["src/**/*.{test,spec}.{ts,tsx}"],
						exclude: ["src/**/*.svelte.{test,spec}.{ts,tsx}"],
					},
				},
			],
		},
		resolve: {
			conditions: ["browser"],
		},
	}),
);
