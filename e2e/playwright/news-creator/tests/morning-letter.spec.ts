import { test, expect, morningLetterBody } from "../src/fixtures.js";
import { expectHeaderContains, expectJsonStatus, expectStatus } from "../../_shared/http.js";
import { todayUTC } from "../../_shared/ids.js";
import { fastapiErrorSchema, morningLetterResponseSchema } from "../src/schemas.js";

/**
 * `POST /v1/morning-letter/generate` — the port of `13-morning-letter.hurl`,
 * plus the degraded branch it never reached.
 *
 * alt-backend's Morning Letter usecase calls this once a day. The usecase here
 * posts to `/api/generate` with `format=MorningLetterContent.model_json_schema()`
 * and unwraps the structured reply; every failure on that path —
 * `TimeoutError`, `JSONDecodeError`, `ValidationError`, `RuntimeError` — is
 * caught and answered as a **200 with an extractive fallback**
 * (usecase/morning_letter_usecase.py:93-125). So status tells you almost
 * nothing here, and `metadata.is_degraded` tells you everything.
 */
test.describe("morning letter", () => {
	test("generates a letter from recap summaries @smoke @contract", async ({ api, seed }) => {
		// A per-test date rather than the Hurl fixture's hard-coded 2026-04-18.
		// `target_date` is `min_length=1` and echoed verbatim, so a service that
		// answered from a cached letter — or from a different request — is
		// visible only when the date is not a constant both sides agree on.
		const targetDate = todayUTC();

		const response = await api.post("/v1/morning-letter/generate", {
			data: morningLetterBody(targetDate),
		});
		const body = await expectJsonStatus(response, 200, morningLetterResponseSchema);
		expectHeaderContains(response, "Content-Type", "application/json");

		expect(body.target_date).toBe(targetDate);
		expect(body.edition_timezone).toBe("Asia/Tokyo");

		// The two assertions that separate "the LLM path ran" from "we silently
		// served the extractive fallback". `_fallback_with_reason` stamps
		// `model="extractive-fallback"` and `is_degraded=True` on its metadata
		// (morning_letter_usecase.py:369-376) — and answers 200 either way. The
		// Hurl scenario asserted neither, so a stack whose structured-output
		// round trip was completely broken would have reported green.
		expect(body.metadata.is_degraded).toBe(false);
		expect(body.metadata.model).not.toBe("extractive-fallback");

		// `summary_length_bullets` is summed across the parsed sections, so it
		// has to agree with the content that came back.
		const bulletCount = body.content.sections.reduce(
			(total, section) => total + section.bullets.length,
			0,
		);
		expect(body.metadata.summary_length_bullets).toBe(bulletCount);

		// Section keys are constrained by a Pydantic `pattern` and the schema
		// repeats it; `_parse_content` filters out sections whose key does not
		// match, so a drifting key is dropped rather than rejected. Asserting at
		// least one survived is what keeps that filter from quietly emptying the
		// letter.
		expect(body.content.sections.length).toBeGreaterThanOrEqual(1);
	});

	test("reports the degraded branch when no recap summaries are supplied @contract", async ({
		api,
	}) => {
		// New coverage. `recap_summaries` is optional, and `generate_letter`
		// treats `None` as the degraded edition: it still calls the LLM (with a
		// different prompt) but re-stamps the metadata with `is_degraded=True`
		// and a fixed `degradation_reason`
		// (usecase/morning_letter_usecase.py:64-92).
		//
		// This branch is what alt-backend renders on a morning when the recap
		// pipeline has not finished, so "the flag is set and the reason says
		// why" is a user-visible contract, not internal bookkeeping. The Hurl
		// suite only ever sent the fully-populated request.
		const targetDate = todayUTC();
		const response = await api.post("/v1/morning-letter/generate", {
			data: morningLetterBody(targetDate, { withRecaps: false }),
		});
		const body = await expectJsonStatus(response, 200, morningLetterResponseSchema);

		expect(body.metadata.is_degraded).toBe(true);
		expect(body.metadata.degradation_reason ?? "").toMatch(/no recap summaries available/i);

		// Still a real letter: the degraded path is a narrower letter, not an
		// empty one. `MorningLetterContent.sections` is `min_length=1`, so an
		// empty result would have been a ValidationError — which would have been
		// caught and turned into the *extractive* fallback, a different state
		// that this assertion distinguishes.
		expect(body.metadata.model).not.toBe("extractive-fallback");
		expect(body.content.sections.length).toBeGreaterThanOrEqual(1);
	});

	test("rejects a request with no overnight groups @contract", async ({ api }) => {
		// New coverage. `overnight_groups` is a required field with no default
		// (domain/models.py MorningLetterRequest); `recap_summaries` is the
		// optional one. Getting those two the wrong way round in a refactor
		// would let a letter be generated from nothing at all.
		const response = await api.post("/v1/morning-letter/generate", {
			data: { target_date: todayUTC(), edition_timezone: "Asia/Tokyo" },
		});
		await expectJsonStatus(response, 422, fastapiErrorSchema);
	});

	test("rejects a group with no articles @contract", async ({ api }) => {
		// New coverage. `MorningLetterGroupInput.articles` is `min_length=1`.
		const response = await api.post("/v1/morning-letter/generate", {
			data: {
				target_date: todayUTC(),
				edition_timezone: "Asia/Tokyo",
				overnight_groups: [{ group_id: "00000000-0000-4000-8000-000000000301", articles: [] }],
			},
		});
		await expectJsonStatus(response, 422, fastapiErrorSchema);
	});

	test("the route lives under the /v1/morning-letter prefix @contract", async ({ api }) => {
		// The router is created with `APIRouter(prefix="/v1/morning-letter")` and
		// registers `POST /generate` on it, so the full path is assembled from
		// two places. 405 proves the assembled path resolves; a 404 would mean
		// either half moved.
		await expectStatus(await api.get("/v1/morning-letter/generate"), 405);
	});
});
