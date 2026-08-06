import { expect, postExtractTags, test } from "../src/fixtures.js";
import { expectJsonStatus } from "../../_shared/http.js";
import { validationErrorSchema } from "../src/schemas.js";

/**
 * `POST /api/v1/extract-tags` request validation — the ports of
 * `05-extract-tags-missing-title.hurl` and
 * `06-extract-tags-missing-content.hurl`, plus the malformed-request shapes
 * neither covered.
 *
 * The contract being pinned is that FastAPI + Pydantic reject at the boundary
 * and the ML pipeline is never reached. That matters beyond tidiness: the
 * handler's `except Exception` turns anything the extractor raises into a
 * 500 (auth_service.py:571-573), so a request that slipped past validation
 * would report as an outage rather than as a client error, and recap-worker
 * would retry it forever.
 *
 * Strengthened throughout: the Hurl scenarios asserted `detail[0].loc` was a
 * collection. Which *field* the error names is the part a caller acts on, so
 * that is what these assert.
 */

/** All `loc` segments in a validation body, flattened for containment checks. */
function locations(detail: readonly { loc: readonly (string | number)[] }[]): string[] {
	return detail.flatMap((entry) => entry.loc.map(String));
}

test.describe("extract-tags validation", () => {
	test("a missing title is rejected with a field-level error", {
		tag: "@contract",
	}, async ({ api }) => {
		const body = await expectJsonStatus(
			await postExtractTags(api, { content: "body-only payload without title field" }),
			422,
			validationErrorSchema,
		);

		// `["body", "title"]` — Pydantic reports the path inside the request,
		// and "body" is what tells a caller the problem is the payload rather
		// than a query parameter or header.
		expect(locations(body.detail)).toContain("title");
		expect(locations(body.detail)).toContain("body");
		expect(body.detail).toHaveLength(1);
	});

	test("a missing content is rejected with a field-level error", {
		tag: "@contract",
	}, async ({ api }) => {
		const body = await expectJsonStatus(
			await postExtractTags(api, { title: "title-only payload" }),
			422,
			validationErrorSchema,
		);

		expect(locations(body.detail)).toContain("content");
		expect(body.detail).toHaveLength(1);
	});

	test("both fields missing produce one error each, not just the first", {
		tag: "@contract",
	}, async ({ api }) => {
		// New. Pydantic v2 collects every failure rather than short-circuiting,
		// which is the difference between a caller fixing its payload in one
		// round trip and in two. A handler rewritten to validate by hand
		// almost always loses this property silently.
		const body = await expectJsonStatus(
			await postExtractTags(api, {}),
			422,
			validationErrorSchema,
		);

		expect(body.detail).toHaveLength(2);
		expect(locations(body.detail)).toEqual(expect.arrayContaining(["title", "content"]));
	});

	test("a non-string title is rejected rather than coerced", {
		tag: "@contract",
	}, async ({ api }) => {
		// New. `title: str` under Pydantic v2 does not coerce an int — v1 did.
		// If it ever silently coerced, `body.title[:50]` in the handler's error
		// logging and the sanitizer's `len(title)` would both be operating on
		// something the caller never sent.
		const body = await expectJsonStatus(
			await postExtractTags(api, { title: 12345, content: "a perfectly valid body" }),
			422,
			validationErrorSchema,
		);

		expect(locations(body.detail)).toContain("title");
	});

	test("a null content is rejected rather than treated as empty", {
		tag: "@contract",
	}, async ({ api }) => {
		// New. `content: str` is not `str | None`, so null is a validation
		// error — not the same request as `""`, which is accepted and lands on
		// the graceful no-signal path (tests/extract-tags.spec.ts). Callers
		// serialising a missing field as null need the 422 to notice.
		const body = await expectJsonStatus(
			await postExtractTags(api, { title: "has a title", content: null }),
			422,
			validationErrorSchema,
		);

		expect(locations(body.detail)).toContain("content");
	});

	test("a body that is not JSON is rejected", { tag: "@contract" }, async ({ api }) => {
		// New. Sent with a JSON Content-Type so the failure is a *parse*
		// failure rather than a content-negotiation one — the shape a
		// truncated or mis-encoded request from a retrying client actually
		// takes. It must be a 4xx: a 500 here would put a malformed client
		// request into the service's error budget.
		const response = await postExtractTags(api, "{ this is not json");
		expect(response.status(), await response.text()).toBe(422);
	});

	test("an empty request body is rejected", { tag: "@contract" }, async ({ api }) => {
		// New. Distinct from `{}`: there is no document at all, which is what a
		// caller that forgot to serialise its payload sends.
		const response = await api.post("/api/v1/extract-tags", {
			headers: { "Content-Type": "application/json" },
		});
		expect(response.status(), await response.text()).toBe(422);
	});
});
