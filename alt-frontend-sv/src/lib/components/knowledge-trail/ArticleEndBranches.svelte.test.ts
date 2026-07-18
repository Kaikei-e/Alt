import { describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import type { BranchData } from "$lib/connect/knowledge_trail";
import ArticleEndBranches from "./ArticleEndBranches.svelte";

function makeBranch(overrides: Partial<BranchData> = {}): BranchData {
	return {
		branchKey: "cluster:u:article:z",
		anchorItemKey: "article:read-end",
		relationKind: "cluster",
		why: "Joins a topic you follow.",
		evidenceRefs: [{ refId: "rust", label: "rust", kind: "tag" }],
		confidence: "plausible",
		targetItemKey: "article:z",
		targetTitle: "Async Rust",
		...overrides,
	};
}

// Wave 10 (D26/D28): the branch's main stage is the article read-end. At most
// two proposals surface here, subordinate to the content itself.
describe("ArticleEndBranches", () => {
	it("renders nothing when there are no branches", async () => {
		render(ArticleEndBranches, {
			props: { branches: [], onResolve: vi.fn() },
		});
		expect(page.getByTestId("article-end-branch").elements()).toHaveLength(0);
	});

	it("renders a branch with its relation-kind label, why, evidence and confidence", async () => {
		render(ArticleEndBranches, {
			props: { branches: [makeBranch()], onResolve: vi.fn() },
		});
		await expect
			.element(page.getByTestId("article-end-branch"))
			.toBeInTheDocument();
		await expect
			.element(page.getByText("Joins a topic you follow", { exact: true }))
			.toBeInTheDocument();
		await expect
			.element(page.getByTestId("article-end-branch"))
			.toHaveTextContent("plausible");
	});

	it("caps visible branches at 2 even when more are supplied", async () => {
		const branches = Array.from({ length: 4 }, (_, i) =>
			makeBranch({
				branchKey: `cluster:u:article:${i}`,
				targetItemKey: `article:${i}`,
				targetTitle: `Title ${i}`,
			}),
		);
		render(ArticleEndBranches, { props: { branches, onResolve: vi.fn() } });
		expect(page.getByTestId("article-end-branch").elements()).toHaveLength(2);
	});

	it("Take this path calls onResolve as taken with the target item key", async () => {
		const onResolve = vi.fn();
		render(ArticleEndBranches, {
			props: { branches: [makeBranch()], onResolve },
		});
		await page.getByTestId("branch-take").click();
		expect(onResolve).toHaveBeenCalledWith(
			"cluster:u:article:z",
			"taken",
			"article:z",
		);
	});

	it("Dismiss opens a one-tap reason row instead of resolving immediately", async () => {
		const onResolve = vi.fn();
		render(ArticleEndBranches, {
			props: { branches: [makeBranch()], onResolve },
		});
		await page.getByTestId("branch-dismiss").click();
		expect(onResolve).not.toHaveBeenCalled();
		await expect
			.element(page.getByTestId("branch-dismiss-reason-not_following_topic"))
			.toBeInTheDocument();
		await expect
			.element(page.getByTestId("branch-dismiss-reason-already_known"))
			.toBeInTheDocument();
		await expect
			.element(page.getByTestId("branch-dismiss-reason-wrong_relation"))
			.toBeInTheDocument();
		await expect
			.element(page.getByTestId("branch-dismiss-plain"))
			.toBeInTheDocument();
	});

	it("picking a reason resolves as dismissed carrying that reason", async () => {
		const onResolve = vi.fn();
		render(ArticleEndBranches, {
			props: { branches: [makeBranch()], onResolve },
		});
		await page.getByTestId("branch-dismiss").click();
		await page.getByTestId("branch-dismiss-reason-already_known").click();
		expect(onResolve).toHaveBeenCalledWith(
			"cluster:u:article:z",
			"dismissed",
			undefined,
			"already_known",
		);
	});

	it("Just dismiss resolves as dismissed with no reason", async () => {
		const onResolve = vi.fn();
		render(ArticleEndBranches, {
			props: { branches: [makeBranch()], onResolve },
		});
		await page.getByTestId("branch-dismiss").click();
		await page.getByTestId("branch-dismiss-plain").click();
		expect(onResolve).toHaveBeenCalledWith(
			"cluster:u:article:z",
			"dismissed",
			undefined,
			undefined,
		);
	});
});
