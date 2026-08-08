import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

/**
 * Regression gate for the push-only service worker (ADR-000962).
 *
 * The worker exists for one reason: iOS delivers Web Push only to a Home
 * Screen web app and only through a service worker. It is deliberately not a
 * caching worker, and that is load-bearing rather than stylistic.
 *
 * This app already carries a defence-in-depth stack against stale
 * `/_app/immutable/*` chunks — the failure that surfaces on iOS Safari as
 * "Cannot Open the Page" and kills the whole tab (ADR-000898, ADR-000902).
 * Every layer of it assumes the network is authoritative: the HTML is served
 * `no-cache, must-revalidate`, `updated.check()` polls the deployed version,
 * and the `app.html` bootstrap reloads on a chunk 404. A worker that answered
 * `fetch` would sit in front of all of them and could serve an evicted module
 * map indefinitely, turning a self-healing failure into a permanent one.
 *
 * ADR-000898 additionally dismissed a Cache Storage hypothesis on the grounds
 * that this repository registered no service worker at all. Registering one
 * moved that dismissal from an observed fact onto a constraint — and a
 * constraint with nothing checking it is a comment. This file is the check.
 *
 * The E2E suite asserts the worker reaches `activated`; it cannot assert the
 * *absence* of a handler, because a caching worker activates just as happily.
 */

const workerSource = readFileSync(
	fileURLToPath(new URL("./service-worker.ts", import.meta.url)),
	"utf-8",
);

/** Comments explain why the handler is absent, so they must not count as usage. */
const workerCode = workerSource
	.replace(/\/\*[\s\S]*?\*\//g, "")
	.replace(/^\s*\/\/.*$/gm, "");

describe("service worker stays push-only", () => {
	it("registers no fetch handler", () => {
		expect(
			workerCode,
			"a fetch handler would put this worker in front of every layer of the stale-chunk defence (ADR-000898 / ADR-000902)",
		).not.toMatch(/addEventListener\s*\(\s*["'`]fetch["'`]/);
		expect(workerCode).not.toMatch(/onfetch\s*=/);
	});

	it("uses no Cache Storage", () => {
		// Without a cache there is no stale module map for the worker to hold,
		// which is what keeps ADR-000898's dismissal true in substance.
		expect(workerCode).not.toMatch(/\bcaches\b/);
		expect(workerCode).not.toMatch(/\bCacheStorage\b/);
	});

	it("does not precache the build manifest", () => {
		// `$service-worker` exposes `build`, `files` and `prerendered` — the
		// ingredients of an app-shell precache. Importing them has no purpose in
		// a worker that never answers a request.
		expect(workerCode).not.toMatch(/from\s+["'`]\$service-worker["'`]/);
	});

	it("still handles the two events it exists for", () => {
		// The inverse guard: a worker that lost these would pass every
		// assertion above while delivering nothing.
		expect(workerCode).toMatch(/addEventListener\s*\(\s*["'`]push["'`]/);
		expect(workerCode).toMatch(
			/addEventListener\s*\(\s*["'`]notificationclick["'`]/,
		);
	});

	it("always shows a notification for a push it receives", () => {
		// Safari revokes a site's notification permission outright for a push
		// that displays nothing, and Chrome overlays its own "updated in the
		// background" notice. `showNotification` is not optional here.
		expect(workerCode).toMatch(/showNotification\s*\(/);
	});
});
