<script lang="ts">
import { T, useThrelte } from "@threlte/core";
import type { IntersectionEvent } from "@threlte/extras";
import { onDestroy, untrack } from "svelte";
import * as THREE from "three";
import type { TagVersePalette } from "./palette.js";
import {
	applyIncrementalNodeColors,
	applyIncrementalNodeScales,
	type SceneNode,
} from "./sceneModel.js";

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

let currentNodesRef: readonly SceneNode[] | null = null;
let currentHexes: string[] = [];
let currentScales = new Float32Array(0);

function applyInstanceTransform(
	targetMesh: THREE.InstancedMesh,
	index: number,
	scaleValue: number,
) {
	const node = nodes[index];
	if (!node) return;
	_pos.set(node.position[0], node.position[1], node.position[2]);
	_scale.set(scaleValue, scaleValue, scaleValue);
	_matrix.compose(_pos, _quat, _scale);
	targetMesh.setMatrixAt(index, _matrix);
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
	const nodesChanged = currentNodes !== currentNodesRef;

	if (activeMesh.instanceMatrix.count !== requiredCapacity) {
		activeMesh.dispose();
		activeMesh = createInstancedMesh(count);
		mesh = activeMesh;
	} else {
		activeMesh.count = count;
	}

	if (nodesChanged || currentScales.length !== count) {
		currentNodesRef = currentNodes;
		currentScales = new Float32Array(count);
		currentHexes = new Array<string>(count).fill("");
	}

	const changedScaleIndices = applyIncrementalNodeScales(
		currentScales,
		currentNodes,
		s,
		h,
	);
	if (changedScaleIndices.length > 0) {
		for (const idx of changedScaleIndices) {
			applyInstanceTransform(activeMesh, idx, currentScales[idx] ?? 1);
		}
		activeMesh.instanceMatrix.needsUpdate = true;
		if (count > 0) {
			activeMesh.computeBoundingSphere();
		}
	}

	const changedColorIndices = applyIncrementalNodeColors(
		currentHexes,
		currentNodes,
		currentPalette,
		s,
		mixArr,
	);
	if (changedColorIndices.length > 0) {
		for (const idx of changedColorIndices) {
			const hex = currentHexes[idx];
			if (!hex) continue;
			_color.set(hex);
			activeMesh.setColorAt(idx, _color);
		}
		if (activeMesh.instanceColor) {
			activeMesh.instanceColor.needsUpdate = true;
		}
	}

	if (changedScaleIndices.length > 0 || changedColorIndices.length > 0) {
		invalidate();
	}
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
