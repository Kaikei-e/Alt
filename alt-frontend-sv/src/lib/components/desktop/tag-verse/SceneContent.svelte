<script lang="ts">
import { T } from "@threlte/core";
import { interactivity } from "@threlte/extras";
import type { TagCloudItem } from "#lib/connect/index.js";
import CameraRig from "./scene/CameraRig.svelte";
import LabelOverlay from "./scene/LabelOverlay.svelte";
import type { TagVersePalette } from "./scene/palette.js";
import { buildSceneModel, fogWindow } from "./scene/sceneModel.js";
import TagNodes from "./scene/TagNodes.svelte";

interface Props {
	tags: TagCloudItem[];
	onTagSelect: (tagName: string) => void;
	palette: TagVersePalette;
	selectedTag?: string | null;
	autoRotate?: boolean;
	reducedMotion?: boolean;
	resetViewNonce?: number;
}

let {
	tags,
	onTagSelect,
	palette,
	selectedTag = null,
	autoRotate = false,
	reducedMotion = false,
	resetViewNonce = 0,
}: Props = $props();

interactivity();

let hoveredIndex = $state<number | null>(null);

const model = $derived(buildSceneModel(tags));

const selectedIndex = $derived(
	selectedTag !== null ? (model.indexByName.get(selectedTag) ?? null) : null,
);

const selectedNode = $derived(
	selectedIndex !== null ? (model.nodes[selectedIndex] ?? null) : null,
);

const initialFog = $derived(
	fogWindow(model.cloudRadius * 1.5, model.cloudRadius),
);

function handleHoverChange(index: number | null) {
	hoveredIndex = index;
}
</script>

<CameraRig
	cloudRadius={model.cloudRadius}
	{selectedNode}
	{autoRotate}
	{reducedMotion}
	{resetViewNonce}
/>

<T.Color attach="background" args={[palette.paper]} />
<T.Fog attach="fog" args={[palette.paper, initialFog.near, initialFog.far]} />

<TagNodes
	nodes={model.nodes}
	{palette}
	{selectedIndex}
	{hoveredIndex}
	onHoverChange={handleHoverChange}
	onNodeClick={onTagSelect}
/>

<LabelOverlay
	nodes={model.nodes}
	cloudRadius={model.cloudRadius}
	{palette}
	{selectedIndex}
	{hoveredIndex}
/>
