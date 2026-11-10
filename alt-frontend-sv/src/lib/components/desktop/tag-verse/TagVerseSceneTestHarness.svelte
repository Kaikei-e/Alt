<script lang="ts">
import { Canvas } from "@threlte/core";
import { NoToneMapping, type Scene } from "three";
import type { TagCloudEdge, TagCloudItem } from "#lib/connect/articles.js";
import SceneContent from "./SceneContent.svelte";
import { buildPalette } from "./scene/palette.js";
import TagVerseSceneTestFixture from "./TagVerseSceneTestFixture.svelte";

interface Props {
	tags: TagCloudItem[];
	edges?: TagCloudEdge[];
	onTagSelect: (tagName: string) => void;
	onRenderedFrame?: (scene: Scene) => void;
}

let { tags, edges = [], onTagSelect, onRenderedFrame }: Props = $props();

const palette = buildPalette();
</script>

<div class="relative w-full h-full">
	<Canvas
		dpr={[1, 2]}
		renderMode="on-demand"
		toneMapping={NoToneMapping}
		shadows={false}
	>
		<SceneContent {tags} {edges} {onTagSelect} {palette} />
		<TagVerseSceneTestFixture {onRenderedFrame} />
	</Canvas>
</div>
