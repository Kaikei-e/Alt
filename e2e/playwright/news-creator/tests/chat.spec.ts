import { test, expect, chatBody } from "../src/fixtures.js";
import { expectHeaderContains, expectJsonStatus, expectStatus } from "../../_shared/http.js";
import { env } from "../src/env.js";
import { chatResponseSchema, fastapiErrorSchema } from "../src/schemas.js";

/**
 * `POST /api/chat` with `stream: false` — the port of
 * `08-chat-non-stream.hurl`.
 *
 * The chat proxy exists so Ask Augur's requests acquire a *high-priority*
 * semaphore slot instead of hitting Ollama directly and being starved by batch
 * summarization. The response body is the upstream's, forwarded through
 * `JSONResponse(content=result)` with nothing added or removed — which is why
 * this file asserts the envelope and deliberately says nothing about
 * `message.content`'s internal structure.
 */
test.describe("chat proxy (non-streaming)", () => {
	test("proxies a non-streaming completion @smoke @contract", async ({ api, seed }) => {
		const response = await api.post("/api/chat", {
			data: chatBody(`stub chat smoke test ${seed.token}`, false),
		});
		const body = await expectJsonStatus(response, 200, chatResponseSchema);
		expectHeaderContains(response, "Content-Type", "application/json");

		// `model` echoes what the caller asked for, because the proxy copies
		// `request.model` into the payload only when set. With
		// MODEL_ROUTING_ENABLED=false in this slice there is no router in the
		// path to substitute a bucket model, so a different value here means the
		// proxy rewrote a field it is supposed to pass through.
		expect(body.model).toBe(env.stubModel);

		// `done: true` on the non-streaming path is the caller's only signal that
		// the completion is whole — the Morning Letter usecase reads the content
		// straight out and parses it as JSON, so a truncated `done: false` reply
		// would surface as a parse error three layers away from here.
		expect(body.done).toBe(true);
	});

	test("rejects an empty messages list @contract", async ({ api }) => {
		// New coverage. `ChatRequest.messages` is `Field(min_length=1)`. Ollama
		// answers a request with no messages by hallucinating from the system
		// prompt alone; refusing at the boundary is what keeps that from
		// reaching the GPU (handler/chat_handler.py ChatRequest).
		const response = await api.post("/api/chat", {
			data: { model: env.stubModel, messages: [], stream: false },
		});
		await expectJsonStatus(response, 422, fastapiErrorSchema);
	});

	test("rejects a non-boolean stream flag under strict mode @contract", async ({ api, seed }) => {
		// New coverage. `ChatRequest` sets `strict=True`, so `"false"` is not
		// coerced to `False`. This one matters more than it looks: `stream`
		// selects between two completely different response media types
		// (`application/json` vs `application/x-ndjson`), and a coerced value
		// would hand a client the wrong framing with a 200 on it.
		const response = await api.post("/api/chat", {
			data: { model: env.stubModel, messages: [{ role: "user", content: seed.token }], stream: "false" },
		});
		await expectJsonStatus(response, 422, fastapiErrorSchema);
	});

	test("rejects a message with a non-string content @contract", async ({ api }) => {
		// New coverage. `ChatMessage` is strict+frozen too, so the nested model
		// is validated with the same rigour as the outer one — worth pinning,
		// because a nested `dict[str, Any]` escape hatch is the usual way strict
		// validation gets quietly widened.
		const response = await api.post("/api/chat", {
			data: { model: env.stubModel, messages: [{ role: "user", content: 42 }], stream: false },
		});
		await expectJsonStatus(response, 422, fastapiErrorSchema);
	});

	test("GET is not allowed on the chat proxy @contract", async ({ api }) => {
		await expectStatus(await api.get("/api/chat"), 405);
	});
});
