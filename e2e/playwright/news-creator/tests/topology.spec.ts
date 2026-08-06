import { test, expect } from "../src/fixtures.js";
import { expectJsonStatus, expectStatus } from "../../_shared/http.js";
import { expectConnectionRefused } from "../../_shared/net.js";
import { env } from "../src/env.js";
import { fastapiErrorSchema } from "../src/schemas.js";

/**
 * Topology — entirely new coverage.
 *
 * news-creator sits directly in front of an Ollama instance that, in
 * production, is a GPU host with model-management endpoints on it. main.py
 * mounts exactly two Ollama-shaped routes — `POST /api/generate` (via
 * `create_generate_router`) and `POST /api/chat` (via `create_chat_router`) —
 * and both go through the HybridPrioritySemaphore. Everything else in Ollama's
 * API is deliberately *not* proxied.
 *
 * That is a security boundary, not a coverage gap. `POST /api/pull` on a real
 * Ollama downloads an arbitrary model onto the GPU host; `DELETE /api/delete`
 * removes one; `/api/embeddings` is unmetered GPU time outside the semaphore.
 * A well-meaning "just forward the whole Ollama API" refactor would open all
 * of them at once, and nothing else in this suite — or in the unit tests,
 * which never see the router table — would notice.
 *
 * Every negative below asserts **404**, never 401/403: a 401 would mean the
 * route is registered and only a middleware stands in front of it. 404 is the
 * only status that says "this surface is not here".
 */

/** Ollama endpoints news-creator must not republish. */
const UNPROXIED_OLLAMA_ROUTES = [
	// Model management on the GPU host. `/api/pull` is the sharpest of these:
	// it takes a model name and fetches it from a registry.
	"/api/pull",
	"/api/push",
	"/api/delete",
	"/api/create",
	"/api/copy",
	// `/api/tags` is called *by* OllamaGateway.list_models() against the
	// upstream, and its result is republished — filtered — through /health.
	// The raw endpoint itself is not a news-creator route.
	"/api/tags",
	"/api/show",
	"/api/ps",
	// Unmetered GPU work: neither embedding endpoint passes through the
	// priority semaphore, so proxying one would let a caller starve
	// summarization without ever appearing in /queue/status.
	"/api/embeddings",
	"/api/embed",
] as const;

test.describe("the Ollama admin surface is not proxied", () => {
	for (const path of UNPROXIED_OLLAMA_ROUTES) {
		test(`POST ${path} → 404 @authz @contract`, async ({ api }) => {
			const response = await api.post(path, { data: {} });
			await expectStatus(response, 404);
		});
	}

	test("a mounted route answers, which is what gives the 404s meaning @contract", async ({
		api,
	}) => {
		// The control. If an unregistered path and a registered one both answered
		// 404, the assertions above would prove nothing. `POST /api/generate`
		// with an empty body resolves and is rejected by Pydantic — 422, not 404.
		const response = await api.post("/api/generate", { data: {} });
		await expectStatus(response, 422);
		expect(response.status(), "the control route must not 404").not.toBe(404);
	});
});

test.describe("listener topology", () => {
	test("nothing is bound on the __main__ development port @authz", async ({ api }) => {
		// `main.py`'s `if __name__ == "__main__"` block runs
		// `uvicorn.run(app, host="0.0.0.0", port=8001)`, while the shipped image
		// binds 11434 (news-creator/Dockerfile:75). An entrypoint that regressed
		// to `python main.py` would listen on 8001 instead — and would still pass
		// a naive "does the container run" check while every caller in the mesh
		// got connection refused on 11434.
		//
		// Hurl could not express this at all: an entry that fails to reach the
		// server is a run failure there, full stop.
		await expectConnectionRefused(
			api,
			`${env.unboundURL}/health`,
			"news-creator serves only the port its Dockerfile CMD binds; main.py's " +
				"__main__ development port must have nothing on it",
		);
	});

	test("an unknown path answers FastAPI's 404 envelope @contract", async ({ api }) => {
		// Pins the error envelope a caller sees for a wrong path. FastAPI answers
		// `{"detail": "Not Found"}`, not the `{"error": ...}` Alt's Go services
		// use — worth fixing in a test because a client that switches on the
		// envelope shape breaks silently when it changes.
		const body = await expectJsonStatus(await api.get("/no-such-route"), 404, fastapiErrorSchema);
		expect(body.detail).toBe("Not Found");
	});
});
