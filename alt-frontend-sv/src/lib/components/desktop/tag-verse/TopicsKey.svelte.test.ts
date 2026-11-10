import { describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import { buildPalette } from "./scene/palette.js";
import { buildSceneModel, buildTopicClusters } from "./scene/sceneModel.js";
import TopicsKey from "./TopicsKey.svelte";

describe("TopicsKey", () => {
	it("renders topic buttons with accessible names and calls the emphasis callback with cluster id on focus and null on blur", async () => {
		const onFocusCluster = vi.fn();
		const model = buildSceneModel([
			{
				tagName: "AI",
				articleCount: 60,
				positionX: 1,
				positionY: 0,
				positionZ: 0,
				clusterId: 1,
			},
			{
				tagName: "agents",
				articleCount: 40,
				positionX: 2,
				positionY: 0,
				positionZ: 0,
				clusterId: 1,
			},
			{
				tagName: "rust",
				articleCount: 50,
				positionX: -1,
				positionY: 0,
				positionZ: 0,
				clusterId: 2,
			},
			{
				tagName: "tokio",
				articleCount: 30,
				positionX: -2,
				positionY: 0,
				positionZ: 0,
				clusterId: 2,
			},
		]);
		const clusters = buildTopicClusters(model.nodes, buildPalette());

		render(TopicsKey, {
			props: {
				clusters,
				focusedClusterId: null,
				onFocusCluster,
			},
		});

		const region = page.getByRole("region", { name: "Topics" });
		await expect.element(region).toBeInTheDocument();

		const aiBtn = page.getByRole("button", {
			name: "Highlight topic: AI · agents",
		});
		await expect.element(aiBtn).toBeInTheDocument();

		const aiEl = (await aiBtn.element()) as HTMLButtonElement;
		aiEl.focus();
		expect(onFocusCluster).toHaveBeenCalledWith(1);
		expect(aiEl.getAttribute("data-emphasized")).toBe("true");

		aiEl.blur();
		expect(onFocusCluster).toHaveBeenLastCalledWith(null);
	});

	it("is absent when there are no clusters", () => {
		const { container } = render(TopicsKey, {
			props: {
				clusters: [],
				focusedClusterId: null,
				onFocusCluster: vi.fn(),
			},
		});

		expect(container.querySelector('[role="region"]')).toBeNull();
		expect(container.querySelectorAll("button").length).toBe(0);
	});
});
