<script lang="ts">
import { useThrelte } from "@threlte/core";
import { interactivity } from "@threlte/extras";
import { onMount } from "svelte";
import * as THREE from "three";
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
	onSceneCreated?: (scene: THREE.Scene) => void;
}

let {
	tags,
	edges: _edges = [],
	onTagSelect,
	palette,
	selectedTag = null,
	selectionSource = "canvas",
	focusedClusterId: _focusedClusterId = null,
	autoRotate = false,
	reducedMotion = false,
	resetViewNonce = 0,
	onSceneCreated,
}: Props = $props();

const { scene, renderer, invalidate } = useThrelte();

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

$effect.pre(() => {
	const paperHex = palette.paper;
	const { near, far } = initialFog;

	if (scene.background instanceof THREE.Color) {
		scene.background.set(paperHex);
	} else {
		scene.background = new THREE.Color(paperHex);
	}

	if (scene.fog instanceof THREE.Fog) {
		scene.fog.color.set(paperHex);
	} else {
		scene.fog = new THREE.Fog(paperHex, near, far);
	}

	renderer.setClearColor(paperHex, 1);
	invalidate();
});

function handleHoverChange(index: number | null) {
	hoveredIndex = index;
}
</script>

<CameraRig
	cloudRadius={model.cloudRadius}
	{selectedNode}
	{selectionSource}
	{autoRotate}
	{reducedMotion}
	{resetViewNonce}
/>

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
