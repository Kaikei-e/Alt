import { defineApiSuite } from "../_shared/config.js";

/**
 * alt-backend API E2E — the largest suite in the fleet.
 *
 * Drives HTTP only: REST on :9000, Connect-RPC on :9101, the loopback
 * operator listener on :9102, and the shared ops listener on :9110. No spec
 * touches `page`, `browser` or `context`, so Playwright never launches a
 * browser and the runner needs no browser binaries at all.
 *
 * Everything about retries, reporters, sharding and the `toPass` backstop
 * lives in `defineApiSuite` — see `_shared/config.ts` for why each is what it
 * is. Only the facts genuinely local to alt-backend are here.
 */
export default defineApiSuite({
	service: "alt-backend",

	/**
	 * The ceiling is not CPU but alt-backend's DB pool (DB_MAX_CONNS=10 in the
	 * staging slice); 6 keeps a request per worker comfortably inside it while
	 * still cutting wall-clock roughly 6x against a serial run.
	 */
	workers: 6,

	globalSetup: "./setup/global-setup.ts",

	projects: [
		/**
		 * The rate-limit specs drain and then observe a shared token bucket —
		 * global state, unlike everything else in this suite, which is isolated
		 * by naming. They get a single-worker project of their own
		 * (`TestProject.workers`, Playwright 1.52+) rather than a `serial`
		 * describe that would still run alongside five other workers hammering
		 * the same backend.
		 */
		{
			name: "api",
			testIgnore: /rate-limit\.spec\.ts$/,
		},
		{
			name: "shared-state",
			testMatch: /rate-limit\.spec\.ts$/,
			workers: 1,
		},
	],
});
