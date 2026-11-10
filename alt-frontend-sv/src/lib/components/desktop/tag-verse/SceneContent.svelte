<script lang="ts">
import { T, useThrelte } from "@threlte/core";
import { interactivity } from "@threlte/extras";
import { onMount } from "svelte";
import type { Scene } from "three";
import type { TagCloudEdge, TagCloudItem } from "#lib/connect/articles.js";
import CameraRig from "./scene/CameraRig.svelte";
import LabelOverlay from "./scene/LabelOverlay.svelte";
import type { TagVersePalette } from "./scene/palette.js";
import {
	buildSceneModel,
	fogWindow,
	type SelectionSource,
} from "./scene/sceneModel.js";
import TagNodes from "./scene/TagNodes.svelte";

interface Props {
	tags: TagCloudItem[];
	edges?: TagCloudEdge[];
	onTagSelect: (tagName: string) => void;
	palette: TagVersePalette;
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
	edges: _edges = [],
	onTagSelect,
	palette,
	selectedTag = null,
	selectionSource: _selectionSource = "canvas",
	focusedClusterId: _focusedClusterId = null,
	autoRotate = false,
	reducedMotion = false,
	resetViewNonce = 0,
	onSceneCreated,
}: Props = $props();

const { scene } = useThrelte();

onMount(() => {
	onSceneCreated?.(scene);
});

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
