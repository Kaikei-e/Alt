import { describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import { buildPalette } from "./scene/palette.js";
import { buildTopicClusters } from "./scene/sceneModel.js";
import TopicsKey from "./TopicsKey.svelte";

describe("TopicsKey", () => {
	it("renders topic buttons with accessible names and calls the emphasis callback on hover and focus", async () => {
		const onFocusCluster = vi.fn();
		const clusters = buildTopicClusters(
			[
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
			],
			buildPalette(),
		);

		const view = render(TopicsKey, {
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
		const rustBtn = page.getByRole("button", {
			name: "Highlight topic: rust · tokio",
		});
		await expect.element(aiBtn).toHaveAttribute("data-emphasized", "false");
		await expect.element(rustBtn).toHaveAttribute("data-emphasized", "false");

		await rustBtn.hover();
		expect(onFocusCluster).toHaveBeenCalledWith(2);

		const aiEl = (await aiBtn.element()) as HTMLButtonElement;
		aiEl.focus();
		expect(onFocusCluster).toHaveBeenCalledWith(1);

		await view.rerender({
			clusters,
			focusedClusterId: 1,
			onFocusCluster,
		});
		await expect.element(aiBtn).toHaveAttribute("data-emphasized", "true");
		await expect.element(rustBtn).toHaveAttribute("data-emphasized", "false");

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
