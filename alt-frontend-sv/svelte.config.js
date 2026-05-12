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
		version: {
			name: resolveVersionName(),
		},
	},
};

export default config;
