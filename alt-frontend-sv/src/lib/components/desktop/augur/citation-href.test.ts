import { describe, expect, it } from "vitest";
import { citationHref, type CitationLinkInput } from "./citation-href";

const summaryRefId = "11111111-1111-4111-8111-111111111111";
const articleRefId = "22222222-2222-4222-8222-222222222222";
const externalUrl = "https://example.test/posts/x";
const bareUuid = "33333333-3333-4333-8333-333333333333";

function input(overrides: Partial<CitationLinkInput>): CitationLinkInput {
	return {
		kind: "UNSPECIFIED",
		url: "",
		refId: "",
		...overrides,
	};
}

describe("citationHref", () => {
	it("returns /articles/<refId> when kind is SUMMARY", () => {
		expect(citationHref(input({ kind: "SUMMARY", refId: summaryRefId }))).toBe(
			`/articles/${summaryRefId}`,
		);
	});

	it("returns /articles/<refId> when kind is ARTICLE", () => {
		expect(citationHref(input({ kind: "ARTICLE", refId: articleRefId }))).toBe(
			`/articles/${articleRefId}`,
		);
	});

	it("returns the external url when kind is WEB", () => {
		expect(citationHref(input({ kind: "WEB", url: externalUrl }))).toBe(
			externalUrl,
		);
	});

	it("returns undefined when WEB has an empty url", () => {
		expect(citationHref(input({ kind: "WEB", url: "" }))).toBeUndefined();
	});

	it("returns undefined when SUMMARY/ARTICLE has an empty refId", () => {
		expect(citationHref(input({ kind: "SUMMARY", refId: "" }))).toBeUndefined();
		expect(citationHref(input({ kind: "ARTICLE", refId: "" }))).toBeUndefined();
	});

	it("returns undefined when kind is UNSPECIFIED, regardless of url contents", () => {
		expect(citationHref(input({ kind: "UNSPECIFIED" }))).toBeUndefined();
		expect(
			citationHref(input({ kind: "UNSPECIFIED", url: bareUuid })),
		).toBeUndefined();
		expect(
			citationHref(input({ kind: "UNSPECIFIED", url: externalUrl })),
		).toBeUndefined();
	});

	it("treats a legacy bare-UUID url with no kind as unlinkable", () => {
		// Legacy citations persisted before the proto change have `url` set to a
		// raw UUID and no `kind`. The helper must NOT emit an href, otherwise the
		// browser resolves the bare UUID relative to /augur/<conversation_id>
		// and re-introduces the bug this change fixes.
		expect(
			citationHref(input({ kind: "UNSPECIFIED", url: bareUuid })),
		).toBeUndefined();
	});

	it("returns external URL for WEB even when refId is also populated", () => {
		// Defensive: if the upstream over-populates both fields, kind wins.
		expect(
			citationHref(
				input({ kind: "WEB", url: externalUrl, refId: articleRefId }),
			),
		).toBe(externalUrl);
	});
});
