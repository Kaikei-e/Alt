import { test, expect } from "../src/fixtures.js";
import { expectHeaderContains, expectJsonStatus } from "../../_shared/http.js";
import { env } from "../src/env.js";
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

	test("GET /health needs no credential", { tag: "@smoke" }, async ({ playwright }) => {
		// The shared `api` fixture sends only a Content-Type, so the assertion
		// above already runs anonymously — but it does so incidentally. This
		// makes it the claim: a bare context, no headers of any kind. The
		// container's own HEALTHCHECK (Dockerfile.tag-generator) and the
		// compose healthcheck are both unauthenticated `urllib` calls, so an
		// auth middleware growing over this route would deadlock startup, not
		// merely break a test.
		const bare = await playwright.request.newContext();
		try {
			await expectJsonStatus(await bare.get(`${env.baseURL}/health`), 200, healthSchema);
		} finally {
			await bare.dispose();
		}
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
