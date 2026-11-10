import { Color, Fog, Scene, type WebGLRenderer } from "three";
import { describe, expect, it, vi } from "vitest";
import { paintPaper } from "./paperBackground.js";

function createRendererStub(): Pick<WebGLRenderer, "setClearColor"> {
	return {
		setClearColor: vi.fn(),
	};
}

describe("paintPaper", () => {
	it("sets scene.background to a Color equal to the paper hex and calls renderer.setClearColor with alpha 1", () => {
		const scene = new Scene();
		const renderer = createRendererStub();

		paintPaper(scene, renderer, "#faf9f7", { near: 40, far: 160 });

		expect(scene.background).toBeInstanceOf(Color);
		expect((scene.background as Color).getHexString()).toBe(
			new Color("#faf9f7").getHexString(),
		);
		expect(renderer.setClearColor).toHaveBeenCalledWith("#faf9f7", 1);
	});

	it("updates an existing background Color in place", () => {
		const scene = new Scene();
		const existingBg = new Color("#000000");
		scene.background = existingBg;
		const renderer = createRendererStub();

		paintPaper(scene, renderer, "#faf9f7", { near: 40, far: 160 });

		expect(scene.background).toBe(existingBg);
		expect(existingBg.getHexString()).toBe(new Color("#faf9f7").getHexString());
	});

	it("creates a Fog with the given near/far and paper colour when absent, and only recolours when present", () => {
		const scene = new Scene();
		const renderer = createRendererStub();

		paintPaper(scene, renderer, "#faf9f7", { near: 42, far: 180 });

		expect(scene.fog).toBeInstanceOf(Fog);
		const createdFog = scene.fog as Fog;
		expect(createdFog.near).toBe(42);
		expect(createdFog.far).toBe(180);
		expect(createdFog.color.getHexString()).toBe(
			new Color("#faf9f7").getHexString(),
		);

		paintPaper(scene, renderer, "#f3efe6", { near: 99, far: 300 });

		expect(scene.fog).toBe(createdFog);
		expect(createdFog.near).toBe(42);
		expect(createdFog.far).toBe(180);
		expect(createdFog.color.getHexString()).toBe(
			new Color("#f3efe6").getHexString(),
		);
	});

	it("updates background, fog colour, and renderer clear colour on a second call with a different paper hex", () => {
		const scene = new Scene();
		const renderer = createRendererStub();

		paintPaper(scene, renderer, "#faf9f7", { near: 50, far: 200 });
		paintPaper(scene, renderer, "#e8e4dc", { near: 50, far: 200 });

		const expectedHex = new Color("#e8e4dc").getHexString();
		expect((scene.background as Color).getHexString()).toBe(expectedHex);
		expect((scene.fog as Fog).color.getHexString()).toBe(expectedHex);
		expect(renderer.setClearColor).toHaveBeenNthCalledWith(1, "#faf9f7", 1);
		expect(renderer.setClearColor).toHaveBeenNthCalledWith(2, "#e8e4dc", 1);
	});
});
