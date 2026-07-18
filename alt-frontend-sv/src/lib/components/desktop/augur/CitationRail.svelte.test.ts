import { page } from "@vitest/browser/context";
import { render } from "vitest-browser-svelte";
import { describe, expect, it } from "vitest";
import CitationRail, { type Citation } from "./CitationRail.svelte";

/**
 * Regression for Svelte each_key_duplicate: WEB (refId "") + legacy (refId "")
 * both evaluated to key "" under `??`, aborting AugurChat mount in E2E.
 */
describe("CitationRail", () => {
	it("renders WEB and legacy citations when both have empty RefID", async () => {
		const externalUrl = "https://example.test/posts/google-health-fitbit";
		const legacyBareUuid = "44444444-4444-4444-8444-444444444444";
		const citations: Citation[] = [
			{
				URL: "",
				Title: "summary",
				Kind: "SUMMARY",
				RefID: "22222222-2222-4222-8222-222222222222",
			},
			{
				URL: "",
				Title: "article",
				Kind: "ARTICLE",
				RefID: "33333333-3333-4333-8333-333333333333",
			},
			{
				URL: externalUrl,
				Title: "Reference",
				Kind: "WEB",
				RefID: "",
			},
			{
				URL: legacyBareUuid,
				Title: "legacy",
				Kind: "UNSPECIFIED",
				RefID: "",
			},
		];

		render(CitationRail, { props: { citations } });

		await expect.element(page.getByText("summary")).toBeInTheDocument();
		await expect.element(page.getByText("article")).toBeInTheDocument();
		await expect.element(page.getByText("Reference")).toBeInTheDocument();
		await expect.element(page.getByText("legacy")).toBeInTheDocument();
	});

	it("renders related citations with empty RefID without key collision", async () => {
		const citations: Citation[] = [
			{
				URL: "",
				Title: "Direct",
				Kind: "ARTICLE",
				RefID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
			},
		];
		const relatedCitations: Citation[] = [
			{ URL: "", Title: "Neighbor A", Kind: "ARTICLE", RefID: "" },
			{ URL: "", Title: "Neighbor B", Kind: "ARTICLE", RefID: "" },
		];

		render(CitationRail, { props: { citations, relatedCitations } });

		await expect.element(page.getByText("Neighbor A")).toBeInTheDocument();
		await expect.element(page.getByText("Neighbor B")).toBeInTheDocument();
	});
});
