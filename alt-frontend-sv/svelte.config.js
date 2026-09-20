import { execSync } from "node:child_process";
import adapter from "@sveltejs/adapter-node";
import { vitePreprocess } from "@sveltejs/vite-plugin-svelte";

// SvelteKit falls back to a full-page navigation whenever the client's app
// version differs from the server's. The framework default for `version.name`
// is a build timestamp, so *every* rebuild — even a no-op CI rebuild — looks
// like a new deploy and forces extra full document loads on the next client
// navigation. On iOS Safari each of those reloads is another chance to hit the
// "could not connect to the server" stale-connection failure. Pin the version
// to an identifier that only changes when the code actually changes: an
// explicit build id, then the git commit SHA (CI passes it as an env var
// because `.git` is dockerignored), then the timestamp as a last resort.
//
// One `vite build` re-imports this file once per pass (client, then server),
// each time with a cache-busting query, so the resolved value has to be
// memoised somewhere that outlives a module instance. Without that, the
// timestamp branch hands each pass a different name: the client bundle is
// compiled against `globalThis.__sveltekit_<hash of pass A>` while the SSR'd
// HTML defines `<hash of pass B>`, and the client runtime dies dereferencing
// the missing global on the first statement of `start()` — before hydration,
// before `onMount`, on every route. `process.env` is the only channel the
// passes share. (SvelteKit's own default avoids this by evaluating its
// `Date.now()` once at module scope, inside a module Node caches.)
const RESOLVED_VERSION_ENV = "ALT_RESOLVED_BUILD_VERSION";

function resolveVersionName() {
	const memoised = process.env[RESOLVED_VERSION_ENV];
	if (memoised) return memoised;

	const resolved = readVersionName();
	process.env[RESOLVED_VERSION_ENV] = resolved;
	return resolved;
}

function readVersionName() {
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
		// Hardened CSP defense-in-depth policy:
		// - script-src 'self' (auto mode generates nonces on adapter-node)
		// - default-src 'self', object-src 'none', base-uri 'self', frame-ancestors 'self'
		// - style-src 'self' 'unsafe-inline' https://fonts.googleapis.com (required for Svelte inline styles & Google Fonts)
		// - font-src 'self' https://fonts.gstatic.com (required for Google Fonts in app.html)
		// - img-src 'self' data: https: (required for proxied/direct OG images & avatars)
		// - connect-src 'self' (for same-origin Connect-RPC & SSE endpoints)
		// - form-action 'self' (for Ory Kratos and internal form submissions)
		csp: {
			mode: "auto",
			directives: {
				"default-src": ["self"],
				"script-src": ["self"],
				"style-src": ["self", "unsafe-inline", "https://fonts.googleapis.com"],
				"font-src": ["self", "https://fonts.gstatic.com"],
				"img-src": ["self", "data:", "https:"],
				"connect-src": ["self"],
				"object-src": ["none"],
				"base-uri": ["self"],
				"frame-ancestors": ["self"],
				"form-action": ["self"],
			},
		},
	},
};

export default config;
