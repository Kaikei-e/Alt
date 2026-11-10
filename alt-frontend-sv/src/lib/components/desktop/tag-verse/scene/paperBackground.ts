import { Color, Fog, type Scene, type WebGLRenderer } from "three";
import type { FogWindow } from "./sceneModel.js";

export function paintPaper(
	scene: Scene,
	renderer: Pick<WebGLRenderer, "setClearColor">,
	paperHex: string,
	fogWindow: FogWindow,
): void {
	if (scene.background instanceof Color) {
		scene.background.set(paperHex);
	} else {
		scene.background = new Color(paperHex);
	}

	if (scene.fog instanceof Fog) {
		scene.fog.color.set(paperHex);
	} else {
		scene.fog = new Fog(paperHex, fogWindow.near, fogWindow.far);
	}

	renderer.setClearColor(paperHex, 1);
}
