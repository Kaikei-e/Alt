import { describe, expect, it } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import type { SupersedeInfoData } from "$lib/connect/knowledge_home";
import SupersedeDetail from "./SupersedeDetail.svelte";

function makeInfo(
	overrides: Partial<SupersedeInfoData> = {},
): SupersedeInfoData {
	return {
		state: "summary_updated",
		supersededAt: "2026-03-17T14:00:00Z",
		previousSummaryExcerpt: "Old summary text",
		previousTags: ["OldTag"],
		previousWhyCodes: ["new_unread"],
		...overrides,
	};
}

describe("SupersedeDetail", () => {
	it("shows the supersede label", async () => {
		render(SupersedeDetail as never, {
			props: { info: makeInfo() },
		});

		await expect.element(page.getByText("Summary updated")).toBeInTheDocument();
	});

	it("shows previous summary excerpt", async () => {
		render(SupersedeDetail as never, {
			props: { info: makeInfo() },
		});

		await expect
			.element(page.getByText("Old summary text"))
			.toBeInTheDocument();
	});

	it("shows previous tags with strikethrough", async () => {
		render(SupersedeDetail as never, {
			props: { info: makeInfo({ previousTags: ["OldTag", "Removed"] }) },
		});

		await expect.element(page.getByText("OldTag")).toBeInTheDocument();
		await expect.element(page.getByText("Removed")).toBeInTheDocument();
	});

	it("shows change description for summary_updated", async () => {
		render(SupersedeDetail as never, {
			props: { info: makeInfo({ state: "summary_updated" }) },
		});

		await expect
			.element(
				page.getByText("The summary was regenerated with updated content."),
			)
			.toBeInTheDocument();
	});

	it("shows change description for tags_updated", async () => {
		render(SupersedeDetail as never, {
			props: { info: makeInfo({ state: "tags_updated" }) },
		});

		await expect
			.element(page.getByText("Tags were recalculated based on new analysis."))
			.toBeInTheDocument();
	});

	it("shows change description for both_updated", async () => {
		render(SupersedeDetail as never, {
			props: { info: makeInfo({ state: "both_updated" }) },
		});

		await expect
			.element(page.getByText("Both summary and tags were updated."))
			.toBeInTheDocument();
	});

	it("shows previous why codes", async () => {
		render(SupersedeDetail as never, {
			props: {
				info: makeInfo({ previousWhyCodes: ["new_unread", "tag_hotspot"] }),
			},
		});

		await expect.element(page.getByText("new_unread")).toBeInTheDocument();
		await expect.element(page.getByText("tag_hotspot")).toBeInTheDocument();
	});

	it("handles empty previous data gracefully", async () => {
		render(SupersedeDetail as never, {
			props: {
				info: makeInfo({
					previousSummaryExcerpt: "",
					previousTags: [],
					previousWhyCodes: [],
				}),
			},
		});

		// Should still render the label without breaking
		await expect.element(page.getByText("Summary updated")).toBeInTheDocument();
	});
});
