import { Color, Fog, type Scene } from "three";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-svelte";

vi.mock("three-perf", () => ({
	ThreePerf: class {},
}));
vi.mock("three-viewport-gizmo", () => ({
	ViewportGizmo: class {},
}));

import TagVerseScene from "./TagVerseScene.svelte";

describe("TagVerseScene", () => {
	it("sets scene.background and scene.fog colour to Alt-Paper #faf9f7 after mount", async () => {
		let capturedScene: Scene | null = null;

		const view = render(TagVerseScene, {
			props: {
				tags: [
					{
						tagName: "svelte",
						articleCount: 10,
						positionX: 0,
						positionY: 0,
						positionZ: 0,
						clusterId: 1,
					},
				],
				edges: [],
				onTagSelect: vi.fn(),
				onSceneCreated: (scene: Scene) => {
					capturedScene = scene;
				},
			},
		});

		await vi.waitFor(() => {
			expect(capturedScene).not.toBeNull();
		});

		await view.rerender({
			tags: [
				{
					tagName: "svelte",
					articleCount: 10,
					positionX: 0,
					positionY: 0,
					positionZ: 0,
					clusterId: 1,
				},
				{
					tagName: "rust",
					articleCount: 20,
					positionX: 25,
					positionY: 10,
					positionZ: -15,
					clusterId: 2,
				},
			],
			edges: [],
			onTagSelect: vi.fn(),
			onSceneCreated: (scene: Scene) => {
				capturedScene = scene;
			},
		});
		await new Promise((r) => setTimeout(r, 50));

		const expectedPaper = new Color("#faf9f7").getHexString();
		expect(capturedScene?.background).toBeInstanceOf(Color);
		expect((capturedScene?.background as Color).getHexString()).toBe(
			expectedPaper,
		);
		expect(capturedScene?.fog).toBeInstanceOf(Fog);
		expect((capturedScene?.fog as Fog).color.getHexString()).toBe(
			expectedPaper,
		);

		view.unmount();
	});
});
