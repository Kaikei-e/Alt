<script lang="ts">
import { T, useThrelte } from "@threlte/core";
import type { IntersectionEvent } from "@threlte/extras";
import { onDestroy, untrack } from "svelte";
import * as THREE from "three";
import type { TagVersePalette } from "./palette.js";
import type { SceneNode } from "./sceneModel.js";

interface Props {
	nodes: readonly SceneNode[];
	palette: TagVersePalette;
	selectedIndex: number | null;
	hoveredIndex: number | null;
	onHoverChange: (index: number | null) => void;
	onNodeClick: (tagName: string) => void;
}

let {
	nodes,
	palette,
	selectedIndex,
	hoveredIndex,
	onHoverChange,
	onNodeClick,
}: Props = $props();

const { invalidate } = useThrelte();

const HIGHLIGHT_SCALE = 1.15;

const geometry = new THREE.SphereGeometry(1, 24, 16);
const material = new THREE.MeshBasicMaterial({ color: "#ffffff" });

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
) {
	const node = nodes[index];
	if (!node) return;
	const hex = isHighlighted
		? currentPalette.accent
		: currentPalette.ramp[node.colorBucket];
	_color.set(hex);
	targetMesh.setColorAt(index, _color);
}

let prevHovered: number | null = null;
let prevSelected: number | null = null;

$effect(() => {
	const currentNodes = nodes;
	const currentPalette = palette;
	const h = untrack(() => hoveredIndex);
	const s = untrack(() => selectedIndex);
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
		applyInstanceTransform(activeMesh, i, isHighlighted);
		applyInstanceColor(activeMesh, i, isHighlighted, currentPalette);
	}

	activeMesh.instanceMatrix.needsUpdate = true;
	if (activeMesh.instanceColor) {
		activeMesh.instanceColor.needsUpdate = true;
	}
	if (count > 0) {
		activeMesh.computeBoundingSphere();
	}

	prevHovered = h;
	prevSelected = s;
	invalidate();
});

$effect(() => {
	const nextHovered = hoveredIndex;
	const nextSelected = selectedIndex;
	if (nextHovered === prevHovered && nextSelected === prevSelected) {
		return;
	}

	const activeMesh = untrack(() => mesh);
	const currentPalette = untrack(() => palette);
	const count = activeMesh.count;
	const dirty = new Set<number>();

	if (prevHovered !== null && prevHovered < count) dirty.add(prevHovered);
	if (nextHovered !== null && nextHovered < count) dirty.add(nextHovered);
	if (prevSelected !== null && prevSelected < count) dirty.add(prevSelected);
	if (nextSelected !== null && nextSelected < count) dirty.add(nextSelected);

	for (const idx of dirty) {
		const isHighlighted = idx === nextHovered || idx === nextSelected;
		applyInstanceTransform(activeMesh, idx, isHighlighted);
		applyInstanceColor(activeMesh, idx, isHighlighted, currentPalette);
	}

	if (dirty.size > 0) {
		activeMesh.instanceMatrix.needsUpdate = true;
		if (activeMesh.instanceColor) {
			activeMesh.instanceColor.needsUpdate = true;
		}
		if (count > 0) {
			activeMesh.computeBoundingSphere();
		}
	}

	prevHovered = nextHovered;
	prevSelected = nextSelected;
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
