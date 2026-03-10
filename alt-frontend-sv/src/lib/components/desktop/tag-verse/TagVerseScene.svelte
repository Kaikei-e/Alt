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
	const renderer = new WebGPURenderer({ canvas, antialias: true });
	// Bind dispose so Threlte's unbound `const dispose = renderer.dispose; dispose()` works.
	// WebGPURenderer.dispose is a prototype method that needs `this`.
	renderer.dispose = renderer.dispose.bind(renderer);
	return renderer;
}
</script>

<div class="w-full h-full" role="presentation">
	<Canvas {createRenderer}>
		<SceneContent {tags} {onTagSelect} />
	</Canvas>
</div>
