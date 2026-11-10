<script lang="ts">
import { T, useThrelte } from "@threlte/core";
import { interactivity } from "@threlte/extras";
import { onDestroy, untrack } from "svelte";
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
	DIR_LIGHT_INTENSITY,
	fogWindow,
	HEMI_LIGHT_INTENSITY,
	KEY_LIGHT_DIRECTION,
	type SelectionSource,
	updateEdgeBufferAlphas,
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
	occludedRightPx?: number;
	autoRotate?: boolean;
	reducedMotion?: boolean;
	resetViewNonce?: number;
}

let {
	tags,
	edges = [],
	onTagSelect,
	palette,
	selectedTag = null,
	selectionSource = "canvas",
	focusedClusterId = null,
	occludedRightPx = 0,
	autoRotate = false,
	reducedMotion = false,
	resetViewNonce = 0,
}: Props = $props();

const { scene, renderer, invalidate } = useThrelte();

interactivity();

let hoveredIndex = $state<number | null>(null);

const model = $derived(buildSceneModel(tags, palette));
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

const hemiLight = new THREE.HemisphereLight(
	"#ffffff",
	"#faf9f7",
	HEMI_LIGHT_INTENSITY,
);
const dirLight = new THREE.DirectionalLight("#ffffff", DIR_LIGHT_INTENSITY);
dirLight.position.set(
	KEY_LIGHT_DIRECTION[0],
	KEY_LIGHT_DIRECTION[1],
	KEY_LIGHT_DIRECTION[2],
);

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
	const initialAlphas = untrack(() => focusEmphasis.edgeAlphas);

	const { positions, colors } = buildEdgeBuffers(
		currentNodes,
		currentEdges,
		currentPalette,
		initialAlphas,
	);
	edgeGeometry.setAttribute(
		"position",
		new THREE.BufferAttribute(positions, 3),
	);
	edgeGeometry.setAttribute("color", new THREE.BufferAttribute(colors, 4));
	edgeSegments.visible = currentEdges.length > 0;
	invalidate();
});

$effect(() => {
	const alphas = focusEmphasis.edgeAlphas;
	const colorAttr = edgeGeometry.getAttribute("color");
	if (colorAttr instanceof THREE.BufferAttribute) {
		const changed = updateEdgeBufferAlphas(
			colorAttr.array as Float32Array,
			alphas,
		);
		if (changed.length > 0) {
			colorAttr.needsUpdate = true;
			invalidate();
		}
	}
});

onDestroy(() => {
	hemiLight.dispose();
	dirLight.dispose();
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
	{occludedRightPx}
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
