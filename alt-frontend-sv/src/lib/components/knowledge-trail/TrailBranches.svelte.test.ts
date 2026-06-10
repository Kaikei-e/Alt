import { describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import type { BranchData } from "$lib/connect/knowledge_trail";
import TrailBranches from "./TrailBranches.svelte";

function makeBranch(overrides: Partial<BranchData> = {}): BranchData {
	return {
		branchKey: "cluster:u:article:z",
		anchorItemKey: "article:a",
		relationKind: "cluster",
		why: "Joins a topic you follow.",
		evidenceRefs: [{ refId: "rust", label: "rust", kind: "tag" }],
		confidence: "plausible",
		targetItemKey: "article:z",
		targetTitle: "Async Rust",
		...overrides,
	};
}

describe("TrailBranches", () => {
	it("renders a branch with its relation-kind label", async () => {
		render(TrailBranches, {
			props: { branches: [makeBranch()], onResolve: vi.fn() },
		});
		await expect.element(page.getByTestId("trail-branch")).toBeInTheDocument();
		await expect
			.element(page.getByText("Joins a topic you follow", { exact: true }))
			.toBeInTheDocument();
	});

	it("Take this path resolves the branch as taken", async () => {
		const onResolve = vi.fn();
		render(TrailBranches, { props: { branches: [makeBranch()], onResolve } });
		await page.getByTestId("branch-take").click();
		expect(onResolve).toHaveBeenCalledWith("cluster:u:article:z", "taken");
	});

	it("Dismiss resolves the branch as dismissed", async () => {
		const onResolve = vi.fn();
		render(TrailBranches, { props: { branches: [makeBranch()], onResolve } });
		await page.getByTestId("branch-dismiss").click();
		expect(onResolve).toHaveBeenCalledWith("cluster:u:article:z", "dismissed");
	});
});
