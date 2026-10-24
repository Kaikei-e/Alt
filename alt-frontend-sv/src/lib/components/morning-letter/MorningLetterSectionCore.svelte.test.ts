import { page } from "@vitest/browser/context";
import { describe, expect, it } from "vitest";
import { render } from "vitest-browser-svelte";
import type {
	MorningLetterSection,
	MorningLetterSourceProto,
} from "#lib/gen/alt/morning_letter/v2/morning_letter_pb.js";
import MorningLetterSectionCore from "./MorningLetterSectionCore.svelte";

function makeSection(
	overrides: Partial<MorningLetterSection> = {},
): MorningLetterSection {
	return {
		$typeName: "alt.morning_letter.v2.MorningLetterSection",
		key: "top3",
		title: "Top Stories",
		bullets: ["First bullet point", "Second bullet point"],
		genre: "",
		narrative: "Overnight developments across distributed systems.",
		whyReasons: [
			{
				$typeName: "alt.morning_letter.v2.WhyReason",
				code: "in_weekly_recap",
				refId: "",
				tag: "",
			},
			{
				$typeName: "alt.morning_letter.v2.WhyReason",
				code: "tag_hotspot",
				refId: "",
				tag: "rust",
			},
		],
		...overrides,
	};
}

function makeSources(): MorningLetterSourceProto[] {
	return [
		{
			$typeName: "alt.morning_letter.v2.MorningLetterSourceProto",
			letterId: "letter-1",
			sectionKey: "top3",
			articleId: "art-1",
			sourceType: 1,
			position: 0,
		},
		{
			$typeName: "alt.morning_letter.v2.MorningLetterSourceProto",
			letterId: "letter-1",
			sectionKey: "top3",
			articleId: "art-2",
			sourceType: 2,
			position: 1,
		},
	];
}

describe("MorningLetterSectionCore", () => {
	it("renders WhySurfacedBadge when whyReasons length matches bullets 1:1 and omits when mismatched", async () => {
		const { container, unmount } = render(MorningLetterSectionCore, {
			props: {
				section: makeSection(),
				sources: [],
				sourcesLoading: false,
			},
		});

		expect(container.querySelectorAll(".section-bullet-why")).toHaveLength(2);
		unmount();

		const { container: mismatchedContainer } = render(
			MorningLetterSectionCore,
			{
				props: {
					section: makeSection({
						bullets: ["First bullet", "Second bullet"],
						whyReasons: [
							{
								$typeName: "alt.morning_letter.v2.WhyReason",
								code: "in_weekly_recap",
								refId: "",
								tag: "",
							},
						],
					}),
					sources: [],
					sourcesLoading: false,
				},
			},
		);

		expect(
			mismatchedContainer.querySelectorAll(".section-bullet-why"),
		).toHaveLength(0);
	});

	it("renders section narrative and recap vs overnight source badges", async () => {
		const { container } = render(MorningLetterSectionCore, {
			props: {
				section: makeSection({
					narrative: "Editorial synthesis of the morning wire.",
				}),
				sources: makeSources(),
				sourcesLoading: false,
			},
		});

		const narrative = container.querySelector(
			'[data-role="section-narrative"]',
		);
		expect(narrative).not.toBeNull();
		expect(narrative?.textContent?.trim()).toBe(
			"Editorial synthesis of the morning wire.",
		);

		await expect.element(page.getByText("recap")).toBeInTheDocument();
		await expect.element(page.getByText("overnight")).toBeInTheDocument();
	});
});
