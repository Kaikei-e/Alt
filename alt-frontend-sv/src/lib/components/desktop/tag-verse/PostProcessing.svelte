<script lang="ts">
import { useThrelte, useTask } from "@threlte/core";
import { onMount } from "svelte";
import { RenderPipeline } from "three/webgpu";
import { pass } from "three/tsl";
import { bloom } from "three/examples/jsm/tsl/display/BloomNode.js";

const { renderer, scene, camera, renderStage, autoRender } = useThrelte();

let pipeline: RenderPipeline | undefined;

function setupPipeline() {
	const cam = camera.current;
	if (!cam) return;

	const scenePass = pass(scene, cam);
	const scenePassColor = scenePass.getTextureNode("output");
	const bloomPass = bloom(scenePassColor, 1.0, 0.4, 0.3);

	// renderer is WebGPURenderer at runtime (configured via Canvas createRenderer)
	// but useThrelte() types it as WebGLRenderer by default
	// biome-ignore lint/suspicious/noExplicitAny: runtime type is WebGPURenderer
	pipeline = new RenderPipeline(renderer as any);
	pipeline.outputNode = scenePassColor.add(bloomPass);
}

onMount(() => {
	autoRender.set(false);
	setupPipeline();
	return () => {
		if (pipeline) {
			pipeline.dispose();
			pipeline = undefined;
		}
		autoRender.set(true);
	};
});

// Override render loop with pipeline
useTask(
	() => {
		if (pipeline) {
			pipeline.render();
		}
	},
	{ stage: renderStage, autoInvalidate: false },
);
</script>
