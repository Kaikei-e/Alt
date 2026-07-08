import adapter from "@sveltejs/adapter-node";
import { vitePreprocess } from "@sveltejs/vite-plugin-svelte";
import { execSync } from "node:child_process";

// SvelteKit falls back to a full-page navigation whenever the client's app
// version differs from the server's. The framework default for `version.name`
// is a build timestamp, so *every* rebuild — even a no-op CI rebuild — looks
// like a new deploy and forces extra full document loads on the next client
// navigation. On iOS Safari each of those reloads is another chance to hit the
// "could not connect to the server" stale-connection failure. Pin the version
// to an identifier that only changes when the code actually changes: an
// explicit build id, then the git commit SHA (CI passes it as an env var
// because `.git` is dockerignored), then the timestamp as a last resort.
function resolveVersionName() {
	const fromEnv =
		process.env.PUBLIC_BUILD_ID ??
		process.env.GIT_COMMIT_SHA ??
		process.env.GITHUB_SHA;
	if (fromEnv && fromEnv.trim()) return fromEnv.trim();
	try {
		return execSync("git rev-parse --short=12 HEAD", {
			stdio: ["ignore", "pipe", "ignore"],
		})
			.toString()
			.trim();
	} catch {
		return Date.now().toString();
	}
}

/** @type {import('@sveltejs/kit').Config} */
const config = {
	// Consult https://svelte.dev/docs/kit/integrations
	// for more information about preprocessors
	preprocess: vitePreprocess(),

	kit: {
		// adapter-auto only supports some environments, see https://svelte.dev/docs/kit/adapter-auto for a list.
		// If your environment is not supported, or you settled on a specific environment, switch out the adapter.
		// See https://svelte.dev/docs/kit/adapters for more information about adapters.
		adapter: adapter(),
		paths: {
			base: "",
		},
		// Inline route-specific CSS chunks (≤ 5 KB) into the HTML `<style>`
		// block so SvelteKit stops emitting them as both `Link: rel="preload";
		// as="style"` HTTP header entries *and* `<link rel="stylesheet">` head
		// elements. Chrome flags that duplicate as "preloaded using link
		// preload but not used within a few seconds from the window's load
		// event" — see SvelteKit Issue #8549 and DebugBear's "duplicate
		// resource loading" analysis. The big root-layout CSS (~85 KB) is
		// left external for cache efficiency, so one warning may persist
		// until #8549 lands the `modulepreload: 'tag' | 'header'` switch.
		inlineStyleThreshold: 5000,
		version: {
			name: resolveVersionName(),
			// Poll the server for a newer deployed version every 5 min. When the
			// build version changes, `updated.current` from $app/state flips to
			// true and +layout.svelte triggers a reload before the next nav, so
			// the tab cannot end up fetching an evicted /_app/immutable/* chunk
			// and falling into "Cannot Open the Page" on iOS Safari.
			pollInterval: 5 * 60 * 1000,
		},
		// This app renders RSS/upstream-API HTML via {@html} in ~13 places
		// (always through sanitizeHtml/parseMarkdown, but that's the only
		// defense layer if a sanitizer bug ever lets a script through).
		// `script-src: 'self'` blocks that from executing. SvelteKit hashes
		// its own inline scripts/styles (and app.html's) automatically in
		// 'auto' mode, so this doesn't require manual nonce wiring.
		csp: {
			mode: "auto",
			directives: {
				"script-src": ["self"],
			},
		},
	},
};

export default config;
