<script lang="ts">
import { Canvas } from "@threlte/core";
import { onMount } from "svelte";
import { NoToneMapping, type Scene } from "three";
import type { TagCloudEdge, TagCloudItem } from "#lib/connect/articles.js";
import SceneContent from "./SceneContent.svelte";
import {
	buildPalette,
	readPaletteFromElement,
	type TagVersePalette,
} from "./scene/palette.js";
import type { SelectionSource } from "./scene/sceneModel.js";

interface Props {
	tags: TagCloudItem[];
	edges?: TagCloudEdge[];
	onTagSelect: (tagName: string) => void;
	selectedTag?: string | null;
	selectionSource?: SelectionSource;
	focusedClusterId?: number | null;
	autoRotate?: boolean;
	reducedMotion?: boolean;
	resetViewNonce?: number;
	onSceneCreated?: (scene: Scene) => void;
}

let {
	tags,
	edges = [],
	onTagSelect,
	selectedTag = null,
	selectionSource = "canvas",
	focusedClusterId = null,
	autoRotate = false,
	reducedMotion = false,
	resetViewNonce = 0,
	onSceneCreated,
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
			{edges}
			{onTagSelect}
			{selectedTag}
			{selectionSource}
			{focusedClusterId}
			{autoRotate}
			{reducedMotion}
			{resetViewNonce}
			{palette}
			{onSceneCreated}
		/>
	</Canvas>
</div>
