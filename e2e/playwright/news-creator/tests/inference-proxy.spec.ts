import { z } from "zod";
import { test, expect } from "../src/fixtures.js";
import { expectJsonStatus, expectStatus } from "../../_shared/http.js";
import { env } from "../src/env.js";

/**
 * The inference bearer between news-creator and its LLM upstream.
 *
 * Production puts generation-proxy (docker/inference-proxy) in front of the GPU
 * backend and news-creator sends `Authorization: Bearer <inference token>` on
 * every call. The staging slice reproduces that hop in front of the Ollama
 * stub. These tests pin the proxy's half of the contract; news-creator's half
 * is every other spec here that reaches the stub — `/health/deep`'s `ollama`
 * check and each generation that echoes `env.stubModel` — because the proxy
 * answers 401 to anything that arrives without the bearer.
 */

const tagsSchema = z
	.object({
		models: z.array(z.object({ name: z.string() }).passthrough()).min(1),
	})
	.passthrough();

test.describe("generation-proxy", () => {
	test("rejects an LLM call that carries no bearer @authz @contract", async ({ request }) => {
		const response = await request.get(`${env.generationProxyURL}/api/tags`);
		await expectStatus(response, 401);
	});

	test("rejects an LLM call with the wrong bearer @authz @contract", async ({ request }) => {
		const response = await request.get(`${env.generationProxyURL}/api/tags`, {
			headers: { Authorization: "Bearer not-the-inference-token" },
		});
		await expectStatus(response, 401);
	});

	test("forwards a call bearing the token news-creator mounts @smoke @contract", async ({
		request,
	}) => {
		const response = await request.get(`${env.generationProxyURL}/api/tags`, {
			headers: { Authorization: `Bearer ${env.inferenceToken}` },
		});
		const body = await expectJsonStatus(response, 200, tagsSchema);
		expect(body.models.map((model) => model.name)).toContain(env.stubModel);
	});
});
