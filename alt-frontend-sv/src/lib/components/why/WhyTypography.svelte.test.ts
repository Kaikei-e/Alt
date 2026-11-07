import { page } from "@vitest/browser/context";
import { describe, expect, it } from "vitest";
import { render } from "vitest-browser-svelte";
import WhyTypography, { type WhyEvidenceRef } from "./WhyTypography.svelte";

describe("WhyTypography", () => {
	it("renders evidenceRefs that share a refId without key collision", async () => {
		const sharedRefId = "88888888-8888-4888-8888-888888888888";
		const evidenceRefs: WhyEvidenceRef[] = [
			{
				refId: sharedRefId,
				label: "Evidence one",
				kind: "ARTICLE",
			},
			{
				refId: sharedRefId,
				label: "Evidence two",
				kind: "ARTICLE",
			},
		];

		render(WhyTypography, {
			props: {
				text: "Recurring theme across recent articles.",
				evidenceRefs,
			},
		});

		await expect.element(page.getByText("Evidence one")).toBeInTheDocument();
		await expect.element(page.getByText("Evidence two")).toBeInTheDocument();
	});

	it("renders counterEvidenceRefs that share a refId without key collision", async () => {
		const sharedRefId = "99999999-9999-4999-8999-999999999999";
		const counterEvidenceRefs: WhyEvidenceRef[] = [
			{
				refId: sharedRefId,
				label: "Counter one",
				kind: "ARTICLE",
			},
			{
				refId: sharedRefId,
				label: "Counter two",
				kind: "ARTICLE",
			},
		];

		render(WhyTypography, {
			props: {
				text: "Recurring theme across recent articles.",
				counterEvidenceRefs,
			},
		});

		await page.getByRole("button", { name: /COUNTER-EVIDENCE/i }).click();

		await expect.element(page.getByText("Counter one")).toBeInTheDocument();
		await expect.element(page.getByText("Counter two")).toBeInTheDocument();
	});
});
