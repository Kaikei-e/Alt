import { test, expect } from "../src/fixtures.js";
import { env } from "../src/env.js";
import { expectStatus } from "../src/http.js";

/**
 * DoS-protection middleware — entirely new coverage.
 *
 * Nothing in the Hurl suite touched this, and it is the one piece of
 * middleware whose failure mode is *silent success*: a limiter that never
 * triggers looks exactly like a healthy service until the day it is needed.
 *
 * Configuration under test (config.DOSProtectionConfig defaults, which
 * compose.staging.yaml does not override):
 *
 *   RateLimit     100 / WindowSize 1m  → 1.67 req/s sustained
 *   BurstLimit    200                  → the first 200 pass immediately
 *   BlockDuration 5m                   → an IP that empties the bucket is
 *                                        blocked for five minutes
 *
 * Every request in this file carries a `X-Real-IP` that no other test uses, so
 * draining a bucket here cannot block anything else. That isolation is the
 * same mechanism the whole suite relies on (see src/fixtures.ts `clientIP`) —
 * this file is where it is proved rather than assumed.
 *
 * Serial mode: the two tests below drain and then observe the *same* limiter
 * bucket, so they are the one place in this suite where order matters.
 */
test.describe.configure({ mode: "serial" });

/** Enough to clear BurstLimit=200 with room for the refill during the run. */
const BURST_ATTEMPTS = 260;
const CONCURRENCY = 20;

/** An address in RFC 6598 space that no worker fixture can produce. */
const VICTIM_IP = "100.127.255.42";
const BYSTANDER_IP = "100.127.255.43";

test.describe("per-IP rate limiting", () => {
	test("draining the burst blocks the offending address", async ({ playwright }) => {
		test.slow();

		const victim = await playwright.request.newContext({
			baseURL: env.baseURL,
			extraHTTPHeaders: { "X-Real-IP": VICTIM_IP },
		});

		try {
			// `/v1/csrf-token` is public, does no database work and is NOT on the
			// DoS whitelist — the cheapest possible way to empty a token bucket.
			const statuses: number[] = [];
			for (let sent = 0; sent < BURST_ATTEMPTS; sent += CONCURRENCY) {
				const batch = Array.from(
					{ length: Math.min(CONCURRENCY, BURST_ATTEMPTS - sent) },
					() => victim.get("/v1/csrf-token"),
				);
				statuses.push(...(await Promise.all(batch)).map((response) => response.status()));
				if (statuses.includes(429)) break;
			}

			expect(
				statuses.filter((status) => status === 429).length,
				`sent ${statuses.length} requests from one address without ever being ` +
					`rate limited — DOS_PROTECTION_ENABLED may have been turned off, or ` +
					`X-Real-IP stopped being honoured (trustForwardedHeaders)`,
			).toBeGreaterThan(0);

			// The first requests must have succeeded: a limiter that rejects from
			// request one is as broken as one that never rejects.
			expect(statuses[0]).toBe(200);
		} finally {
			await victim.dispose();
		}
	});

	test("the block is scoped to the address, and the whitelist still passes", async ({
		playwright,
	}) => {
		const victim = await playwright.request.newContext({
			baseURL: env.baseURL,
			extraHTTPHeaders: { "X-Real-IP": VICTIM_IP },
		});
		const bystander = await playwright.request.newContext({
			baseURL: env.baseURL,
			extraHTTPHeaders: { "X-Real-IP": BYSTANDER_IP },
		});

		try {
			// Still blocked — BlockDuration is five minutes, far longer than this
			// suite runs, so this needs no sleep and cannot flake on timing.
			await expectStatus(await victim.get("/v1/csrf-token"), 429);

			// A different address is untouched. This is the property that lets six
			// Playwright workers share one backend without fighting over a bucket,
			// and the reason `clientIP` is a worker fixture.
			await expectStatus(await bystander.get("/v1/csrf-token"), 200);

			// `/v1/health` is on WhitelistedPaths, so even a blocked address gets
			// through. Without this, a rate-limited client would also fail the
			// container healthcheck and Prometheus probes — taking the instance
			// out of rotation because one caller misbehaved.
			await expectStatus(await victim.get("/v1/health"), 200);
		} finally {
			await victim.dispose();
			await bystander.dispose();
		}
	});
});
