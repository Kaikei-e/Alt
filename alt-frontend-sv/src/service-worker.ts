/// <reference types="@sveltejs/kit" />
/// <reference lib="esnext" />
/// <reference lib="webworker" />

/**
 * Push-only service worker.
 *
 * **There is deliberately no `fetch` handler here, and adding one is a
 * regression.** This app carries a defence-in-depth stack against stale
 * `/_app/immutable/*` chunks — the failure that surfaces on iOS Safari as
 * "Cannot Open the Page" and kills the whole tab (ADR-000898, ADR-000902).
 * Every layer of it assumes the network is authoritative: the HTML is served
 * `no-cache, must-revalidate`, `updated.check()` polls the deployed version,
 * and the `app.html` bootstrap reloads on a chunk 404. A worker that answered
 * `fetch` would sit in front of all of them and could serve an evicted module
 * map indefinitely — turning a self-healing failure into a permanent one.
 *
 * Registering a worker at all already invalidates one line of reasoning in
 * ADR-000898, which dismissed a Cache Storage hypothesis on the grounds that
 * this repository registered no service worker. Keeping the worker free of
 * `fetch` and of Cache Storage is what keeps that dismissal true in substance.
 *
 * The worker exists for exactly one reason: on iOS, Web Push is delivered only
 * to a Home Screen web app, and only through a service worker.
 */

import { parsePushPayload } from "$lib/push/payload";

const sw = self as unknown as ServiceWorkerGlobalScope;

/**
 * Take over without waiting for every existing tab to close. Safe here only
 * because the worker never answers `fetch`: claiming clients cannot change
 * what any page receives from the network.
 */
sw.addEventListener("install", () => {
	sw.skipWaiting();
});

sw.addEventListener("activate", (event) => {
	event.waitUntil(sw.clients.claim());
});

sw.addEventListener("push", (event) => {
	// Reading the payload must not be able to prevent the notification: a push
	// that displays nothing costs the site its notification permission on
	// Safari, and draws Chrome's "updated in the background" notice. So any
	// failure here degrades to the generic notification rather than propagating.
	let raw: string | null = null;
	try {
		raw = event.data ? event.data.text() : null;
	} catch {
		raw = null;
	}

	const push = parsePushPayload(raw);

	event.waitUntil(
		sw.registration.showNotification(push.title, {
			body: push.body,
			// Replaces an already-displayed notification of the same kind rather
			// than stacking a second one — the daily digest depends on this.
			tag: push.tag,
			icon: "/icons/icon-192.png",
			badge: "/icons/icon-192.png",
			data: { url: push.url },
		}),
	);
});

sw.addEventListener("notificationclick", (event) => {
	event.notification.close();

	const data = event.notification.data as { url?: unknown } | undefined;
	const target = typeof data?.url === "string" ? data.url : "/";

	// Second gate, on the parsed URL rather than on the string. The payload was
	// already filtered by `parsePushPayload`, but `.claude/rules/security-boundaries.md`
	// is explicit that prefix checks alone are not sufficient here — WHATWG
	// parsing normalises `/\` into `//`, so only the resolved origin settles it.
	const resolved = new URL(target, sw.location.origin);
	const href =
		resolved.origin === sw.location.origin ? resolved.href : sw.location.origin;

	event.waitUntil(
		(async () => {
			// `includeUncontrolled` matters: without it a tab opened before the
			// last worker update is invisible here, and the user gets a second
			// window instead of the one they already had.
			const windows = await sw.clients.matchAll({
				type: "window",
				includeUncontrolled: true,
			});

			const existing = windows.find((client) => client.url === href);
			if (existing) {
				await existing.focus();
				return;
			}

			await sw.clients.openWindow(href);
		})(),
	);
});
