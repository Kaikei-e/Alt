<script lang="ts">
import { Canvas } from "@threlte/core";
import { onMount, type Snippet } from "svelte";
import { NoToneMapping } from "three";
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
	occludedRightPx?: number;
	autoRotate?: boolean;
	reducedMotion?: boolean;
	resetViewNonce?: number;
	children?: Snippet;
}

let {
	tags,
	edges = [],
	onTagSelect,
	selectedTag = null,
	selectionSource = "canvas",
	focusedClusterId = null,
	occludedRightPx = 0,
	autoRotate = false,
	reducedMotion = false,
	resetViewNonce = 0,
	children,
}: Props = $props();

let containerRef = $state<HTMLDivElement | undefined>(undefined);
let palette = $state<TagVersePalette>(buildPalette());

const PAPER_GRAIN_DATA_URI =
	"data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='160' height='160'%3E%3Cfilter id='g'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.8' numOctaves='3' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23g)'/%3E%3C/svg%3E";

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
			{occludedRightPx}
			{autoRotate}
			{reducedMotion}
			{resetViewNonce}
			{palette}
		/>
		{@render children?.()}
	</Canvas>
	<div
		aria-hidden="true"
		class="pointer-events-none absolute inset-0"
		style:opacity="0.07"
		style:mix-blend-mode="multiply"
		style:background-image="url(&quot;{PAPER_GRAIN_DATA_URI}&quot;)"
	></div>
</div>
