<script lang="ts">
import { useTask, useThrelte } from "@threlte/core";
import type { Scene } from "three";

interface Props {
	onRenderedFrame?: (scene: Scene) => void;
}

let { onRenderedFrame }: Props = $props();

const { scene, renderStage } = useThrelte();

useTask(
	() => {
		onRenderedFrame?.(scene);
	},
	{ stage: renderStage, autoInvalidate: false },
);
</script>
