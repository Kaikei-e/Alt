/**
 * Articles GetArticleSourceURL contract test (Open CTA recovery path).
 *
 * The Knowledge Loop ACT workspace falls back to this RPC when an article
 * entry's `actTargets[].sourceUrl` is empty (legacy projection row, or a
 * producer-side ADR-879 lookup miss). The wire shape is contract-frozen here
 * so a future regression — empty `sourceUrl` field name drift, or a request
 * body that smuggled a tenant id, etc. — fails before it reaches the BFF.
 *
 * Mirrors the proto-shape style used by `knowledge-loop-contract.test.ts`:
 * vitest-driven `create()` + `toBinary()` + `fromBinary()` round-trips on
 * the generated schemas. The wire-level Hurl scenario at
 * `e2e/hurl/alt-backend/31-articles-source-url.hurl` covers status codes
 * (400 / 404 / 200) end-to-end against alt-backend.
 */
import { describe, it, expect } from "vitest";
import { create, toBinary, fromBinary } from "@bufbuild/protobuf";
import {
	GetArticleSourceURLRequestSchema,
	GetArticleSourceURLResponseSchema,
} from "$lib/gen/alt/articles/v2/articles_pb";

describe("ArticleService.GetArticleSourceURL contract", () => {
	it("request carries article_id only — no tenant or user fields", () => {
		const req = create(GetArticleSourceURLRequestSchema, {
			articleId: "00000000-0000-0000-0000-000000000001",
		});
		expect(req.articleId).toBe("00000000-0000-0000-0000-000000000001");

		// Round-trip via binary keeps the field intact.
		const bytes = toBinary(GetArticleSourceURLRequestSchema, req);
		const decoded = fromBinary(GetArticleSourceURLRequestSchema, bytes);
		expect(decoded.articleId).toBe(req.articleId);

		// Tenant / user must NOT be on the request — they are sourced from JWT.
		// The shape itself enforces this (the schema only has articleId), but we
		// pin the field count so a future "convenience" field cannot be added
		// without an explicit ADR review.
		expect(Object.keys(req).filter((k) => !k.startsWith("$"))).toEqual([
			"articleId",
		]);
	});

	it("response carries source_url and round-trips through binary", () => {
		const res = create(GetArticleSourceURLResponseSchema, {
			sourceUrl: "https://example.com/article-recovered",
		});
		expect(res.sourceUrl).toBe("https://example.com/article-recovered");

		const bytes = toBinary(GetArticleSourceURLResponseSchema, res);
		const decoded = fromBinary(GetArticleSourceURLResponseSchema, bytes);
		expect(decoded.sourceUrl).toBe(res.sourceUrl);
	});

	it("empty source_url is the not_found wire idiom (handler MAY map to 404)", () => {
		// proto3 string default is empty. A handler that returns an empty
		// response body is signaling "not found" without a structured
		// error envelope. Connect-RPC also surfaces this as code=not_found
		// when the handler explicitly returns ConnectError(NotFound).
		const res = create(GetArticleSourceURLResponseSchema, {});
		expect(res.sourceUrl).toBe("");
	});
});
