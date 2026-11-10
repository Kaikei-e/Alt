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

const MAX_INSTANCES = 512;
const HOVER_SCALE = 1.15;

const geometry = new THREE.SphereGeometry(1, 24, 16);
const material = new THREE.MeshBasicMaterial({ color: "#ffffff" });
const mesh = new THREE.InstancedMesh(geometry, material, MAX_INSTANCES);
mesh.setColorAt(0, new THREE.Color("#ffffff"));

const _matrix = new THREE.Matrix4();
const _pos = new THREE.Vector3();
const _quat = new THREE.Quaternion();
const _scale = new THREE.Vector3();
const _color = new THREE.Color();

function applyInstanceTransform(index: number, isHovered: boolean) {
	const node = nodes[index];
	if (!node) return;
	const s = node.radius * (isHovered ? HOVER_SCALE : 1);
	_pos.set(node.position[0], node.position[1], node.position[2]);
	_scale.set(s, s, s);
	_matrix.compose(_pos, _quat, _scale);
	mesh.setMatrixAt(index, _matrix);
}

function applyInstanceColor(
	index: number,
	isHovered: boolean,
	isSelected: boolean,
	currentPalette: TagVersePalette,
) {
	const node = nodes[index];
	if (!node) return;
	const hex =
		isSelected || isHovered
			? currentPalette.accent
			: currentPalette.ramp[node.colorBucket];
	_color.set(hex);
	mesh.setColorAt(index, _color);
}

let prevHovered: number | null = null;
let prevSelected: number | null = null;

$effect(() => {
	const currentNodes = nodes;
	const currentPalette = palette;
	const h = untrack(() => hoveredIndex);
	const s = untrack(() => selectedIndex);
	const count = Math.min(currentNodes.length, MAX_INSTANCES);
	mesh.count = count;

	for (let i = 0; i < count; i++) {
		applyInstanceTransform(i, i === h);
		applyInstanceColor(i, i === h, i === s, currentPalette);
	}

	mesh.instanceMatrix.needsUpdate = true;
	if (mesh.instanceColor) {
		mesh.instanceColor.needsUpdate = true;
	}
	if (count > 0) {
		mesh.computeBoundingSphere();
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

	const currentPalette = untrack(() => palette);
	const count = mesh.count;
	const dirty = new Set<number>();

	if (prevHovered !== null && prevHovered < count) dirty.add(prevHovered);
	if (nextHovered !== null && nextHovered < count) dirty.add(nextHovered);
	if (prevSelected !== null && prevSelected < count) dirty.add(prevSelected);
	if (nextSelected !== null && nextSelected < count) dirty.add(nextSelected);

	const hoverChanged = nextHovered !== prevHovered;

	for (const idx of dirty) {
		if (hoverChanged) {
			applyInstanceTransform(idx, idx === nextHovered);
		}
		applyInstanceColor(
			idx,
			idx === nextHovered,
			idx === nextSelected,
			currentPalette,
		);
	}

	if (hoverChanged && dirty.size > 0) {
		mesh.instanceMatrix.needsUpdate = true;
	}
	if (dirty.size > 0 && mesh.instanceColor) {
		mesh.instanceColor.needsUpdate = true;
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
