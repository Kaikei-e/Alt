<script lang="ts">
import { useTask, useThrelte } from "@threlte/core";
import { onMount } from "svelte";
import * as THREE from "three";
import { type LabelBox, layoutLabels } from "./labelLayout.js";
import type { TagVersePalette } from "./palette.js";
import {
	computeLabelOpacity,
	type FogWindow,
	fogWindow,
	type SceneNode,
} from "./sceneModel.js";

interface Props {
	nodes: readonly SceneNode[];
	cloudRadius: number;
	palette: TagVersePalette;
	selectedIndex: number | null;
	hoveredIndex: number | null;
}

let { nodes, cloudRadius, palette, selectedIndex, hoveredIndex }: Props =
	$props();

const { dom, scene, camera, size, dpr, renderStage, invalidate } = useThrelte();

let canvasEl: HTMLCanvasElement | undefined;
const widthCache = new Map<string, number>();
const _projVec = new THREE.Vector3();
const _camForward = new THREE.Vector3();

const sortedIndicesByCount = $derived.by(() => {
	const indices = Array.from({ length: nodes.length }, (_, i) => i);
	indices.sort(
		(a, b) =>
			(nodes[b]?.articleCount ?? 0) - (nodes[a]?.articleCount ?? 0) || a - b,
	);
	return indices;
});

function portalToDom(el: HTMLCanvasElement) {
	dom.append(el);
	return {
		destroy() {
			el.remove();
		},
	};
}

function measureLabelWidth(
	ctx: CanvasRenderingContext2D,
	tagName: string,
	fontSize: number,
	weight: 600 | 700,
): number {
	const roundedSize = Math.round(fontSize * 10) / 10;
	const key = `${weight}|${roundedSize}|${tagName}`;
	const cached = widthCache.get(key);
	if (cached !== undefined) {
		return cached;
	}
	ctx.font = `${weight} ${roundedSize}px "Source Sans 3", system-ui, sans-serif`;
	const measured = ctx.measureText(tagName).width;
	widthCache.set(key, measured);
	return measured;
}

interface CandidateEntry {
	node: SceneNode;
	screenX: number;
	screenY: number;
	textWidth: number;
	fontSize: number;
	weight: 600 | 700;
	opacity: number;
	isSelected: boolean;
}

function drawLabels() {
	if (!canvasEl) return;
	const cam = camera.current;
	if (!cam) return;

	const cssW = size.current.width;
	const cssH = size.current.height;
	const currentDpr = dpr.current;
	if (cssW <= 0 || cssH <= 0) return;

	const pixelW = Math.max(1, Math.round(cssW * currentDpr));
	const pixelH = Math.max(1, Math.round(cssH * currentDpr));

	if (canvasEl.width !== pixelW || canvasEl.height !== pixelH) {
		canvasEl.width = pixelW;
		canvasEl.height = pixelH;
	}

	const ctx = canvasEl.getContext("2d");
	if (!ctx) return;

	ctx.setTransform(currentDpr, 0, 0, currentDpr, 0, 0);
	ctx.clearRect(0, 0, cssW, cssH);

	if (nodes.length === 0) return;

	cam.updateMatrixWorld();
	cam.getWorldDirection(_camForward);
	const camX = cam.position.x;
	const camY = cam.position.y;
	const camZ = cam.position.z;
	const fwdX = _camForward.x;
	const fwdY = _camForward.y;
	const fwdZ = _camForward.z;

	const activeFog: FogWindow =
		scene.fog instanceof THREE.Fog
			? { near: scene.fog.near, far: scene.fog.far }
			: fogWindow(Math.hypot(camX, camY, camZ), cloudRadius);

	const priorityIndices: number[] = [];
	if (
		selectedIndex !== null &&
		selectedIndex >= 0 &&
		selectedIndex < nodes.length
	) {
		priorityIndices.push(selectedIndex);
	}
	if (
		hoveredIndex !== null &&
		hoveredIndex >= 0 &&
		hoveredIndex < nodes.length &&
		hoveredIndex !== selectedIndex
	) {
		priorityIndices.push(hoveredIndex);
	}
	for (let i = 0; i < sortedIndicesByCount.length; i++) {
		const idx = sortedIndicesByCount[i] as number;
		if (idx !== selectedIndex && idx !== hoveredIndex) {
			priorityIndices.push(idx);
		}
	}

	const candidates: CandidateEntry[] = [];
	const boxes: LabelBox[] = [];

	for (let i = 0; i < priorityIndices.length; i++) {
		const idx = priorityIndices[i] as number;
		const node = nodes[idx];
		if (!node) continue;

		const nodeX = node.position[0];
		const nodeY = node.position[1];
		const nodeZ = node.position[2];

		const viewDepth =
			(nodeX - camX) * fwdX + (nodeY - camY) * fwdY + (nodeZ - camZ) * fwdZ;
		if (viewDepth <= 0) continue;

		const isSelected = idx === selectedIndex;
		const isHovered = idx === hoveredIndex;
		const isForced = isSelected || isHovered;

		const opacity = isForced ? 1 : computeLabelOpacity(viewDepth, activeFog);
		if (opacity <= 0) continue;

		const anchorX = nodeX;
		const anchorY = nodeY + node.radius;
		const anchorZ = nodeZ;

		_projVec.set(anchorX, anchorY, anchorZ).project(cam);
		if (_projVec.z < -1 || _projVec.z > 1) continue;

		const screenX = (_projVec.x * 0.5 + 0.5) * cssW;
		const screenY = (-_projVec.y * 0.5 + 0.5) * cssH;
		if (screenX < 0 || screenX > cssW || screenY < 0 || screenY > cssH) {
			continue;
		}

		const weight: 600 | 700 = isSelected ? 700 : 600;
		const fontSize = Math.round(node.labelFontSize * 10) / 10;
		const textWidth = measureLabelWidth(ctx, node.tagName, fontSize, weight);

		const boxW = textWidth + 6;
		const boxH = fontSize + 4;
		boxes.push({
			x: screenX - boxW * 0.5,
			y: screenY - boxH - 2,
			w: boxW,
			h: boxH,
			forced: isForced,
		});
		candidates.push({
			node,
			screenX,
			screenY,
			textWidth,
			fontSize,
			weight,
			opacity,
			isSelected,
		});
	}

	const accepted = layoutLabels(boxes);

	ctx.textAlign = "center";
	ctx.textBaseline = "bottom";
	ctx.lineJoin = "round";
	ctx.lineCap = "round";
	ctx.lineWidth = 3;
	ctx.strokeStyle = palette.paper;

	for (let i = accepted.length - 1; i >= 0; i--) {
		const candIdx = accepted[i] as number;
		const entry = candidates[candIdx];
		if (!entry) continue;

		const textY = entry.screenY - 3;
		ctx.globalAlpha = entry.opacity;
		ctx.font = `${entry.weight} ${entry.fontSize}px "Source Sans 3", system-ui, sans-serif`;
		ctx.strokeText(entry.node.tagName, entry.screenX, textY);
		ctx.fillStyle = entry.isSelected ? palette.accent : palette.labelInk;
		ctx.fillText(entry.node.tagName, entry.screenX, textY);

		if (entry.isSelected) {
			ctx.fillRect(
				entry.screenX - entry.textWidth * 0.5,
				textY + 2,
				entry.textWidth,
				1,
			);
		}
	}

	ctx.globalAlpha = 1;
}

useTask(drawLabels, { stage: renderStage, autoInvalidate: false });

$effect(() => {
	void $size;
	void $dpr;
	void palette;
	void nodes;
	void selectedIndex;
	void hoveredIndex;
	invalidate();
});

onMount(() => {
	let active = true;
	if (typeof document !== "undefined" && document.fonts) {
		const fontPromises: Promise<unknown>[] = [];
		if (typeof document.fonts.load === "function") {
			fontPromises.push(
				document.fonts.load('600 12px "Source Sans 3"'),
				document.fonts.load('700 12px "Source Sans 3"'),
			);
		}
		if (document.fonts.ready) {
			fontPromises.push(document.fonts.ready);
		}
		Promise.all(fontPromises)
			.then(() => {
				if (active) {
					widthCache.clear();
					invalidate();
				}
			})
			.catch(() => {});
	}
	return () => {
		active = false;
	};
});
</script>

<canvas
	bind:this={canvasEl}
	use:portalToDom
	class="absolute inset-0 w-full h-full pointer-events-none"
	aria-hidden="true"
></canvas>
