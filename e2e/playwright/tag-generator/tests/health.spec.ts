import { test, expect } from "../src/fixtures.js";
import { expectHeaderContains, expectJsonStatus } from "../../_shared/http.js";
import { healthSchema } from "../src/schemas.js";

/**
 * `/health` — the port of `00-setup.hurl` and `01-health-schema.hurl`.
 *
 * The readiness *gate* moved to `setup/global-setup.ts`; what stays here is
 * the *contract*, which is a different claim with a different lifetime. The
 * gate exists so a broken stack fails once; these exist so a handler that
 * changes shape fails in a named test.
 *
 * A 200 is a stronger statement than it looks. `health_check` raises 503 when
 * `_background_service_healthy` is false, and again when any entry in
 * `_consumer_health` is false (auth_service.py:487-508) — so this endpoint is
 * simultaneously the liveness signal for the batch thread and for both Redis
 * Streams consumer threads. The compose healthcheck and this suite's gate
 * both stake everything on that.
 */
test.describe("health", () => {
	test("GET /health reports healthy and names the service", {
		tag: ["@smoke", "@contract"],
	}, async ({ api }) => {
		const response = await api.get("/health");
		await expectJsonStatus(response, 200, healthSchema);

		// FastAPI's default encoder, not proto3-JSON: the whole response is
		// snake_case-native JSON, and `service` is a plain string rather than
		// anything generated. Pinning the Content-Type is what the Hurl
		// scenario did and it still earns its place — a handler switched to
		// PlainTextResponse would keep the status and break every caller.
		expectHeaderContains(response, "Content-Type", "application/json");
	});

	test("GET /health needs no application token", { tag: "@smoke" }, async ({ api }) => {
		// The shared `api` fixture presents the required mTLS client certificate
		// but sends no application credentials, tokens, or peer-identity headers.
		// /health is exempt from the peer allowlist and requires no application-level auth.
		const response = await api.get("/health");
		await expectJsonStatus(response, 200, healthSchema);
	});

	test("GET /health is cheap enough to be a healthcheck", {
		tag: "@smoke",
	}, async ({ api }) => {
		// The compose healthcheck runs every 3s with a 3s timeout. The handler
		// touches only two in-process dicts, so anything approaching that
		// budget means it grew a dependency call — the classic way a health
		// endpoint starts reporting on something it cannot see quickly and
		// begins flapping the container.
		const started = Date.now();
		const response = await api.get("/health");
		const elapsed = Date.now() - started;
		expect(response.status()).toBe(200);
		expect(elapsed, "/health must stay well inside the 3s compose healthcheck timeout").toBeLessThan(
			3_000,
		);
	});
});
