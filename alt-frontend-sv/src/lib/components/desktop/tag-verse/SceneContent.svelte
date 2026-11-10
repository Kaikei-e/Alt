<script lang="ts">
import { T, useThrelte } from "@threlte/core";
import { interactivity } from "@threlte/extras";
import { onDestroy, onMount } from "svelte";
import * as THREE from "three";
import type { TagCloudEdge, TagCloudItem } from "#lib/connect/articles.js";
import CameraRig from "./scene/CameraRig.svelte";
import LabelOverlay from "./scene/LabelOverlay.svelte";
import type { TagVersePalette } from "./scene/palette.js";
import {
	buildEdgeBuffers,
	buildSceneModel,
	buildTopicClusters,
	computeFocusEmphasis,
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
	edges = [],
	onTagSelect,
	palette,
	selectedTag = null,
	selectionSource = "canvas",
	focusedClusterId = null,
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
const topicClusters = $derived(buildTopicClusters(model.nodes, palette));

const selectedIndex = $derived(
	selectedTag !== null ? (model.indexByName.get(selectedTag) ?? null) : null,
);

const selectedNode = $derived(
	selectedIndex !== null ? (model.nodes[selectedIndex] ?? null) : null,
);

const focusIndex = $derived(selectedIndex ?? hoveredIndex);

const focusEmphasis = $derived(
	computeFocusEmphasis(model.nodes, edges, {
		focusIndex,
		focusClusterId: focusedClusterId,
	}),
);

const initialFog = $derived(
	fogWindow(model.cloudRadius * 1.5, model.cloudRadius),
);

// WHY: Under NoToneMapping, a 0.7 hemisphere + 0.3 directional light sum to 1.0 diffuse irradiance on the lit mid-tone, preserving each topic ink hex and its >= 3:1 WCAG contrast on #faf9f7 while shading the dark terminator toward paper.
const hemiLight = new THREE.HemisphereLight("#ffffff", "#faf9f7", 0.7);
const dirLight = new THREE.DirectionalLight("#ffffff", 0.3);
dirLight.position.set(1, 1.5, 1);

const edgeGeometry = new THREE.BufferGeometry();
const edgeMaterial = new THREE.LineBasicMaterial({
	vertexColors: true,
	transparent: true,
	depthWrite: false,
	fog: true,
});
const edgeSegments = new THREE.LineSegments(edgeGeometry, edgeMaterial);
edgeSegments.frustumCulled = false;

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

	hemiLight.groundColor.set(paperHex);
	renderer.setClearColor(paperHex, 1);
	invalidate();
});

$effect(() => {
	const currentNodes = model.nodes;
	const currentEdges = edges;
	const currentPalette = palette;
	const alphas = focusEmphasis.edgeAlphas;

	const { positions, colors } = buildEdgeBuffers(
		currentNodes,
		currentEdges,
		currentPalette,
		alphas,
	);
	edgeGeometry.setAttribute(
		"position",
		new THREE.BufferAttribute(positions, 3),
	);
	edgeGeometry.setAttribute("color", new THREE.BufferAttribute(colors, 4));
	edgeSegments.visible = currentEdges.length > 0;
	invalidate();
});

onDestroy(() => {
	edgeGeometry.dispose();
	edgeMaterial.dispose();
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

<T is={hemiLight} dispose={false} />
<T is={dirLight} dispose={false} />
<T is={edgeSegments} dispose={false} />

<TagNodes
	nodes={model.nodes}
	{palette}
	{selectedIndex}
	{hoveredIndex}
	nodePaperMix={focusEmphasis.nodePaperMix}
	onHoverChange={handleHoverChange}
	onNodeClick={onTagSelect}
/>

<LabelOverlay
	nodes={model.nodes}
	cloudRadius={model.cloudRadius}
	{palette}
	{selectedIndex}
	{hoveredIndex}
	{focusEmphasis}
	{topicClusters}
/>
