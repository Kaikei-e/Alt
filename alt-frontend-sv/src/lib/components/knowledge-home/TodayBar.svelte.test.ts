import { describe, expect, it } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import type { TodayDigestData } from "$lib/connect/knowledge_home";
import TodayBar from "./TodayBar.svelte";

function makeDigest(overrides: Partial<TodayDigestData> = {}): TodayDigestData {
	return {
		date: "2026-03-17",
		newArticles: 42,
		summarizedArticles: 30,
		unsummarizedArticles: 12,
		topTags: ["AI", "Go", "Rust"],
		weeklyRecapAvailable: true,
		eveningPulseAvailable: false,
		needToKnowCount: 0,
		digestFreshness: "fresh",
		lastProjectedAt: null,
		...overrides,
	};
}

describe("TodayBar", () => {
	it("renders digest stats when digest is provided", async () => {
		render(TodayBar as never, { props: { digest: makeDigest() } });

		await expect.element(page.getByText("42")).toBeInTheDocument();
		await expect.element(page.getByText("new")).toBeInTheDocument();
		await expect.element(page.getByText("30")).toBeInTheDocument();
		await expect.element(page.getByText("summarized")).toBeInTheDocument();
	});

	it("renders compact placeholder when digest is null", async () => {
		render(TodayBar as never, { props: { digest: null } });

		await expect
			.element(page.getByText("Today's digest is still being prepared."))
			.toBeInTheDocument();
	});

	it("shows stale indicator when digest freshness is stale", async () => {
		render(TodayBar as never, {
			props: { digest: makeDigest({ digestFreshness: "stale" }) },
		});

		await expect.element(page.getByText("Stale")).toBeInTheDocument();
	});

	it("shows pending count when unsummarized > 0", async () => {
		render(TodayBar as never, {
			props: { digest: makeDigest({ unsummarizedArticles: 5 }) },
		});

		await expect.element(page.getByText("5")).toBeInTheDocument();
		await expect.element(page.getByText("pending")).toBeInTheDocument();
	});

	it("shows fallback message when digestFreshness is unknown", async () => {
		render(TodayBar as never, {
			props: {
				digest: makeDigest({ digestFreshness: "unknown" }),
				serviceQuality: "fallback",
			},
		});

		await expect
			.element(
				page.getByText("Digest section is temporarily unavailable or stale."),
			)
			.toBeInTheDocument();
	});

	it("renders top tags", async () => {
		render(TodayBar as never, {
			props: { digest: makeDigest({ topTags: ["AI", "Go"] }) },
		});

		await expect
			.element(page.getByText("AI", { exact: true }))
			.toBeInTheDocument();
		await expect
			.element(page.getByText("Go", { exact: true }))
			.toBeInTheDocument();
	});

	it("renders morning letter link always", async () => {
		render(TodayBar as never, { props: { digest: makeDigest() } });

		await expect.element(page.getByText("Morning Letter")).toBeInTheDocument();
	});
});
