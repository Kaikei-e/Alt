<script lang="ts">
import { Canvas } from "@threlte/core";
import type { TagCloudItem } from "$lib/connect";
import WebGPURenderer from "three/src/renderers/webgpu/WebGPURenderer.js";
import SceneContent from "./SceneContent.svelte";

interface Props {
	tags: TagCloudItem[];
	onTagSelect: (tagName: string) => void;
}

let { tags, onTagSelect }: Props = $props();

function createRenderer(canvas: HTMLCanvasElement) {
	// Fall back to WebGL2 backend when WebGPU API is unavailable
	// (e.g. Windows ARM64 / Qualcomm Adreno where WebGPU is not enabled by default)
	const forceWebGL = !navigator.gpu;
	const renderer = new WebGPURenderer({ canvas, antialias: true, forceWebGL });
	// Bind dispose so Threlte's unbound `const dispose = renderer.dispose; dispose()` works.
	// WebGPURenderer.dispose is a prototype method that needs `this`.
	const boundDispose = renderer.dispose.bind(renderer);
	// Guard against Three.js WebGPU NodeManager cleanup race condition where
	// internal node references are cleared before material disposal completes.
	renderer.dispose = () => {
		try {
			boundDispose();
		} catch {
			// WebGPURenderer teardown order bug — safe to ignore
		}
	};
	return renderer;
}
</script>

<div class="w-full h-full" role="presentation">
	<Canvas {createRenderer}>
		<SceneContent {tags} {onTagSelect} />
	</Canvas>
</div>
