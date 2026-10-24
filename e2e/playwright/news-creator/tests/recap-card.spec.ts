import { test, expect } from "../src/fixtures.js";
import { expectHeaderContains, expectJsonStatus, expectStatus } from "../../_shared/http.js";
import { uuid } from "../../_shared/ids.js";
import { cardGenerateResponseSchema, fastapiErrorSchema } from "../src/schemas.js";

/**
 * `POST /v1/cards/generate` — topic card generation (`recap_card_handler.py`).
 *
 * `RecapCardUsecase` renders `prompts/recap_card.jinja` (tagged-text sections
 * `【見出し】`, `【何が起きた】`, `【なぜ重要】`, `【出典】`), calls `/api/generate`,
 * parses the tagged output via `parse_card_output`, and enforces the G1
 * Japanese script ratio gate (`ja_ratio >= 0.60`).
 */

function cardRequestBody(jobId: string, candidateId: string) {
	return {
		job_id: jobId,
		candidate_id: candidateId,
		prompt_version: "recap_card.v1",
		items: [
			{
				n: 1,
				feed_id: uuid(),
				title: "ソラリス社、分散ログ基盤の次期バージョンを公開",
				host: "example.com",
				url: "https://example.com/articles/1",
				pub_date: "2026-04-18T00:00:00Z",
				lede: "ソラリス社はメモリ効率を改善した分散ログ収集エンジンを正式に公開した。",
			},
		],
		revision_note: null,
	};
}

test.describe("recap card generation", () => {
	test("generates a structured Japanese card with citation refs @smoke @contract", async ({
		api,
		seed,
	}) => {
		const candidateId = uuid();
		const response = await api.post("/v1/cards/generate", {
			data: cardRequestBody(seed.jobId, candidateId),
		});
		const body = await expectJsonStatus(response, 200, cardGenerateResponseSchema);
		expectHeaderContains(response, "Content-Type", "application/json");
		expect(response.headers()["x-card-rejection"]).toBeUndefined();

		expect(body.card.headline_ja.length).toBeGreaterThan(0);
		expect(body.card.headline_ja.length).toBeLessThanOrEqual(60);
		expect(body.card.what_ja.length).toBeGreaterThanOrEqual(2);
		expect(body.card.what_ja.length).toBeLessThanOrEqual(3);
		expect(body.card.used_refs).toContain(1);
		expect(body.ja_ratio).toBeGreaterThanOrEqual(0.6);
		expect(body.generation.prompt_version).toBe("recap_card.v1");
	});

	test("rejects an empty items list without X-Card-Rejection header @contract", async ({
		api,
		seed,
	}) => {
		// FastAPI request-validation 422 must NOT carry `X-Card-Rejection: 1`
		// (which is reserved for semantic LLM output rejection so callers drop
		// the candidate rather than retrying).
		const response = await api.post("/v1/cards/generate", {
			data: {
				job_id: seed.jobId,
				candidate_id: uuid(),
				items: [],
			},
		});
		await expectJsonStatus(response, 422, fastapiErrorSchema);
		expect(response.headers()["x-card-rejection"]).toBeUndefined();
	});

	test("rejects a malformed job_id UUID @contract", async ({ api }) => {
		const response = await api.post("/v1/cards/generate", {
			data: {
				...cardRequestBody("not-a-uuid", uuid()),
			},
		});
		await expectJsonStatus(response, 422, fastapiErrorSchema);
		expect(response.headers()["x-card-rejection"]).toBeUndefined();
	});

	test("GET is not allowed on /v1/cards/generate @contract", async ({ api }) => {
		await expectStatus(await api.get("/v1/cards/generate"), 405);
	});
});
