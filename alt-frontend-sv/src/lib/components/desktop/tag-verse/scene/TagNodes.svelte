<script lang="ts">
import { T, useThrelte } from "@threlte/core";
import type { IntersectionEvent } from "@threlte/extras";
import { onDestroy, untrack } from "svelte";
import * as THREE from "three";
import {
	clusterInkForTag,
	mixTowardPaper,
	type TagVersePalette,
} from "./palette.js";
import type { SceneNode } from "./sceneModel.js";

interface Props {
	nodes: readonly SceneNode[];
	palette: TagVersePalette;
	selectedIndex: number | null;
	hoveredIndex: number | null;
	nodePaperMix?: Float32Array;
	onHoverChange: (index: number | null) => void;
	onNodeClick: (tagName: string) => void;
}

let {
	nodes,
	palette,
	selectedIndex,
	hoveredIndex,
	nodePaperMix,
	onHoverChange,
	onNodeClick,
}: Props = $props();

const { invalidate } = useThrelte();

const HIGHLIGHT_SCALE = 1.15;

const geometry = new THREE.SphereGeometry(1, 24, 16);
const material = new THREE.MeshLambertMaterial({ color: "#ffffff" });

function createInstancedMesh(count: number): THREE.InstancedMesh {
	const capacity = Math.max(1, count);
	const instanced = new THREE.InstancedMesh(geometry, material, capacity);
	instanced.count = count;
	instanced.setColorAt(0, new THREE.Color("#ffffff"));
	return instanced;
}

let mesh = $state.raw<THREE.InstancedMesh>(
	createInstancedMesh(untrack(() => nodes.length)),
);

const _matrix = new THREE.Matrix4();
const _pos = new THREE.Vector3();
const _quat = new THREE.Quaternion();
const _scale = new THREE.Vector3();
const _color = new THREE.Color();

function applyInstanceTransform(
	targetMesh: THREE.InstancedMesh,
	index: number,
	isHighlighted: boolean,
) {
	const node = nodes[index];
	if (!node) return;
	const s = node.radius * (isHighlighted ? HIGHLIGHT_SCALE : 1);
	_pos.set(node.position[0], node.position[1], node.position[2]);
	_scale.set(s, s, s);
	_matrix.compose(_pos, _quat, _scale);
	targetMesh.setMatrixAt(index, _matrix);
}

function applyInstanceColor(
	targetMesh: THREE.InstancedMesh,
	index: number,
	isHighlighted: boolean,
	currentPalette: TagVersePalette,
	paperMix: number,
) {
	const node = nodes[index];
	if (!node) return;
	const baseHex = isHighlighted
		? currentPalette.accent
		: clusterInkForTag(node.clusterId, node.colorBucket, currentPalette);
	const hex =
		paperMix > 0
			? mixTowardPaper(baseHex, currentPalette.paper, paperMix)
			: baseHex;
	_color.set(hex);
	targetMesh.setColorAt(index, _color);
}

$effect(() => {
	const currentNodes = nodes;
	const currentPalette = palette;
	const h = hoveredIndex;
	const s = selectedIndex;
	const mixArr = nodePaperMix;
	const count = currentNodes.length;
	const requiredCapacity = Math.max(1, count);

	let activeMesh = untrack(() => mesh);
	if (activeMesh.instanceMatrix.count !== requiredCapacity) {
		activeMesh.dispose();
		activeMesh = createInstancedMesh(count);
		mesh = activeMesh;
	} else {
		activeMesh.count = count;
	}

	for (let i = 0; i < count; i++) {
		const isHighlighted = i === h || i === s;
		const paperMix = mixArr && i < mixArr.length ? (mixArr[i] ?? 0) : 0;
		applyInstanceTransform(activeMesh, i, isHighlighted);
		applyInstanceColor(activeMesh, i, isHighlighted, currentPalette, paperMix);
	}

	activeMesh.instanceMatrix.needsUpdate = true;
	if (activeMesh.instanceColor) {
		activeMesh.instanceColor.needsUpdate = true;
	}
	if (count > 0) {
		activeMesh.computeBoundingSphere();
	}

	invalidate();
});

function handlePointerMove(event: IntersectionEvent<PointerEvent>) {
	event.stopPropagation();
	const id = event.instanceId;
	if (id !== undefined && id >= 0 && id < mesh.count) {
		if (hoveredIndex !== id) {
			onHoverChange(id);
		}
		if (typeof document !== "undefined") {
			document.body.style.cursor = "pointer";
		}
	}
}

function handlePointerLeave(event: IntersectionEvent<PointerEvent>) {
	if (event.instanceId === undefined || hoveredIndex === event.instanceId) {
		if (hoveredIndex !== null) {
			onHoverChange(null);
		}
		if (typeof document !== "undefined") {
			document.body.style.cursor = "auto";
		}
	}
}

function handleClick(event: IntersectionEvent<MouseEvent>) {
	event.stopPropagation();
	const id = event.instanceId;
	if (id !== undefined && id >= 0 && id < nodes.length) {
		const node = nodes[id];
		if (node) {
			onNodeClick(node.tagName);
		}
	}
}

onDestroy(() => {
	if (typeof document !== "undefined") {
		document.body.style.cursor = "auto";
	}
	mesh.dispose();
	geometry.dispose();
	material.dispose();
});
</script>

<T
	is={mesh}
	dispose={false}
	onpointermove={handlePointerMove}
	onpointerleave={handlePointerLeave}
	onclick={handleClick}
/>
