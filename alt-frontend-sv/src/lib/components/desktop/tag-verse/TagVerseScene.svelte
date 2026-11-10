<script lang="ts">
import { Canvas } from "@threlte/core";
import { onMount } from "svelte";
import { NoToneMapping } from "three";
import type { TagCloudItem } from "#lib/connect/index.js";
import SceneContent from "./SceneContent.svelte";
import {
	buildPalette,
	readPaletteFromElement,
	type TagVersePalette,
} from "./scene/palette.js";

interface Props {
	tags: TagCloudItem[];
	onTagSelect: (tagName: string) => void;
	selectedTag?: string | null;
	autoRotate?: boolean;
	reducedMotion?: boolean;
	resetViewNonce?: number;
}

let {
	tags,
	onTagSelect,
	selectedTag = null,
	autoRotate = false,
	reducedMotion = false,
	resetViewNonce = 0,
}: Props = $props();

let containerRef = $state<HTMLDivElement | undefined>(undefined);
let palette = $state<TagVersePalette>(buildPalette());

onMount(() => {
	palette = readPaletteFromElement(containerRef);
});
</script>

<div
	bind:this={containerRef}
	class="relative w-full h-full"
	style:background-color={palette.paper}
	role="presentation"
>
	<Canvas
		dpr={[1, 2]}
		renderMode="on-demand"
		toneMapping={NoToneMapping}
		shadows={false}
	>
		<SceneContent
			{tags}
			{onTagSelect}
			{selectedTag}
			{autoRotate}
			{reducedMotion}
			{resetViewNonce}
			{palette}
		/>
	</Canvas>
</div>
