/**
 * Tests for the push payload parser.
 *
 * This lives in `src/lib/` rather than in the service worker because
 * `.svelte-kit/tsconfig.json` excludes `src/service-worker.ts` from
 * `svelte-check` — anything written there is invisible to the type checker and
 * uncollectable by vitest. The worker stays a shell; the decisions live here.
 *
 * The parser has one hard obligation: **it must always yield something
 * displayable.** Safari revokes a site's notification permission outright if a
 * push arrives and no notification is shown, and Chrome overlays its own
 * "This site has been updated in the background" notice. So a malformed
 * payload has to degrade to a generic notification, never to a throw.
 */

import { describe, expect, it } from "vitest";

import { parsePushPayload } from "./payload";

/** The Declarative Web Push envelope Safari understands natively. */
function declarative(notification: Record<string, unknown>): string {
	return JSON.stringify({ web_push: 8030, notification });
}

describe("parsePushPayload", () => {
	it("reads title, body and navigate out of a declarative envelope", () => {
		const parsed = parsePushPayload(
			declarative({
				title: "Recap ready",
				body: "Your 3-day recap finished.",
				navigate: "/recap",
			}),
		);

		expect(parsed.title).toBe("Recap ready");
		expect(parsed.body).toBe("Your 3-day recap finished.");
		expect(parsed.url).toBe("/recap");
	});

	it("collapses repeat notifications of the same kind via a stable tag", () => {
		const first = parsePushPayload(
			declarative({ title: "Today", navigate: "/home", kind: "digest_ready" }),
		);
		const second = parsePushPayload(
			declarative({ title: "Today", navigate: "/home", kind: "digest_ready" }),
		);

		// `tag` replaces an already-displayed notification; without it the daily
		// digest stacks one entry per day on a device that was offline.
		expect(first.tag).toBe(second.tag);
		expect(first.tag).toBe("digest_ready");
	});

	it("gives different kinds different tags so they do not overwrite each other", () => {
		const digest = parsePushPayload(
			declarative({ title: "Today", kind: "digest_ready" }),
		);
		const recap = parsePushPayload(
			declarative({ title: "Recap", kind: "recap_ready" }),
		);

		expect(digest.tag).not.toBe(recap.tag);
	});

	it.each([
		["null payload", null],
		["empty string", ""],
		["not JSON at all", "<html>502</html>"],
		["JSON that is not an object", "42"],
		["object with no notification block", '{"web_push":8030}'],
	])("still yields a displayable notification for %s", (_label, raw) => {
		const parsed = parsePushPayload(raw);

		expect(parsed.title.length).toBeGreaterThan(0);
		expect(parsed.url).toBe("/");
	});

	it("refuses a navigate target that would leave the origin", () => {
		const parsed = parsePushPayload(
			declarative({ title: "Recap ready", navigate: "https://evil.example/x" }),
		);

		// The payload is authenticated by the push subscription, not by us, so a
		// navigate target is treated as untrusted input: an absolute URL would
		// turn a notification tap into an open redirect.
		expect(parsed.url).toBe("/");
	});

	it("refuses a protocol-relative navigate target", () => {
		const parsed = parsePushPayload(
			declarative({ title: "Recap ready", navigate: "//evil.example/x" }),
		);

		expect(parsed.url).toBe("/");
	});

	it("keeps a relative path with a query string intact", () => {
		const parsed = parsePushPayload(
			declarative({ title: "Recap ready", navigate: "/recap?window=3days" }),
		);

		expect(parsed.url).toBe("/recap?window=3days");
	});
});
