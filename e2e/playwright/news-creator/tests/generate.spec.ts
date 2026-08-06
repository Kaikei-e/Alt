import { test, expect, generateBody } from "../src/fixtures.js";
import { expectHeaderContains, expectJsonStatus, expectStatus } from "../../_shared/http.js";
import { env } from "../src/env.js";
import { fastapiErrorSchema, generateResponseSchema } from "../src/schemas.js";

/**
 * `POST /api/generate` — the port of `04-generate.hurl` and the second half of
 * `12-validation-errors.hurl`.
 *
 * The Ollama-compatible pass-through. `generate_handler` does not forward the
 * upstream body: it rebuilds a dict field by field, defaulting `done` to True
 * and `done_reason` to `"stop"`, and appending the three counters only when
 * the upstream supplied them. So this envelope is news-creator's own contract
 * even though it looks like Ollama's.
 */
test.describe("generate pass-through", () => {
	test("answers the Ollama-shaped envelope @smoke @contract", async ({ api, seed }) => {
		const response = await api.post("/api/generate", {
			data: generateBody(
				`次のテキストを 1 行で要約してください。 OpenAI は新しい大規模言語モデルを発表した。 性能は前世代比で大幅に改善されている。 (${seed.token})`,
			),
		});
		const body = await expectJsonStatus(response, 200, generateResponseSchema);
		expectHeaderContains(response, "Content-Type", "application/json");

		// The request named a model; the handler must echo the one the provider
		// actually used. A router that quietly fell back to a different bucket
		// model (MODEL_ROUTING_ENABLED is false in this slice, so it must not)
		// shows up here and nowhere else.
		expect(body.model).toBe(env.stubModel);
	});

	test("strips num_ctx from options without failing the request @contract", async ({
		api,
		seed,
	}) => {
		// New coverage. The handler pops `num_ctx` out of `options` before
		// dispatch so every caller shares one context length and Ollama can reuse
		// its runner (handler/generate_handler.py:52-56). If that pop regressed
		// into a rejection — or into forwarding an unknown key upstream — this is
		// where it surfaces; the Hurl suite never sent `options` at all.
		const response = await api.post("/api/generate", {
			data: {
				...generateBody(`options passthrough probe ${seed.token}`),
				options: { num_ctx: 999_999, temperature: 0.1 },
			},
		});
		await expectJsonStatus(response, 200, generateResponseSchema);
	});

	test("accepts a num_predict override in options @contract", async ({ api, seed }) => {
		// New coverage. `num_predict` is popped and coerced with `int()`; a
		// non-integer is logged and ignored rather than raised
		// (generate_handler.py:57-70). Both branches must answer 200 — the
		// tolerant branch especially, because a caller that sends a string is a
		// bug we degrade through rather than a request we reject.
		const response = await api.post("/api/generate", {
			data: {
				...generateBody(`num_predict probe ${seed.token}`),
				options: { num_predict: "not-an-int" },
			},
		});
		await expectJsonStatus(response, 200, generateResponseSchema);
	});

	test("rejects an empty prompt @contract", async ({ api }) => {
		// Port of `12-validation-errors.hurl` case 2. `GenerateRequest.prompt` is
		// `min_length=1`, so this is Pydantic's 422 and not the handler's 400 —
		// the distinction the Hurl comment got wrong in prose and right in the
		// assertion.
		const response = await api.post("/api/generate", {
			data: { prompt: "", model: env.stubModel, stream: false },
		});
		await expectJsonStatus(response, 422, fastapiErrorSchema);
	});

	test("rejects a prompt past the 240,000-character cap with a 400 @contract", async ({ api }) => {
		// New coverage, and the one guard the Hurl suite named in a comment but
		// never exercised. `generate_handler` raises `ValueError` above
		// MAX_PROMPT_LENGTH_CHARS = 240_000, which its own `except ValueError`
		// turns into a 400 (generate_handler.py:60-74). The point of the cap is
		// queue hygiene: a 60k-token prompt occupies the single semaphore slot
		// for minutes and starves every other caller, so "the request is
		// rejected before it reaches acquire()" is the behaviour under test.
		//
		// 400, specifically. A 422 would mean the check moved into Pydantic and
		// the handler's own ValueError path went dead.
		const response = await api.post("/api/generate", {
			data: { prompt: "あ".repeat(240_001), model: env.stubModel, stream: false },
		});
		const body = await expectJsonStatus(response, 400, fastapiErrorSchema);
		expect(typeof body.detail === "string" ? body.detail : "").toMatch(/prompt too long/i);
	});

	test("accepts a prompt exactly at the cap @contract", async ({ api }) => {
		// The other side of the boundary. The guard is `> MAX_PROMPT_LENGTH_CHARS`,
		// so 240,000 exactly must go through — an off-by-one that turned it into
		// `>=` would start rejecting legitimate long-context work, and nothing
		// else in the suite would notice.
		const response = await api.post("/api/generate", {
			data: { prompt: "あ".repeat(240_000), model: env.stubModel, stream: false },
		});
		await expectStatus(response, 200);
	});

	test("GET is not allowed on the generate endpoint @contract", async ({ api }) => {
		// 405 rather than 404: the path resolves, so the generation router is
		// mounted (main.py `create_generate_router`).
		await expectStatus(await api.get("/api/generate"), 405);
	});
});
