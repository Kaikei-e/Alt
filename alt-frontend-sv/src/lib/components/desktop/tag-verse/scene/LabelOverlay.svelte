<script lang="ts">
import { useTask, useThrelte } from "@threlte/core";
import { onMount } from "svelte";
import * as THREE from "three";
import { type LabelBox, layoutLabels } from "./labelLayout.js";
import type { TagVersePalette } from "./palette.js";
import {
	buildRegionLabelCandidates,
	computeLabelOpacity,
	type FocusEmphasis,
	type FogWindow,
	fogWindow,
	resolveNodeLabelTypography,
	type SceneNode,
	type TopicClusterSummary,
} from "./sceneModel.js";

interface Props {
	nodes: readonly SceneNode[];
	cloudRadius: number;
	palette: TagVersePalette;
	selectedIndex: number | null;
	hoveredIndex: number | null;
	focusEmphasis?: FocusEmphasis;
	topicClusters?: readonly TopicClusterSummary[];
}

let {
	nodes,
	cloudRadius,
	palette,
	selectedIndex,
	hoveredIndex,
	focusEmphasis,
	topicClusters = [],
}: Props = $props();

const { dom, scene, camera, size, dpr, renderStage, invalidate } = useThrelte();

let canvasEl: HTMLCanvasElement | undefined;
let fontsReady = typeof document === "undefined" || !document.fonts;
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
	text: string,
	fontSize: number,
	weight: 600 | 700,
	fontFamily: string,
	letterSpacingEm = 0,
): number {
	const roundedSize = Math.round(fontSize * 10) / 10;
	const key = `${fontFamily}|${weight}|${roundedSize}|${letterSpacingEm}|${text}`;
	const cached = widthCache.get(key);
	if (cached !== undefined) {
		return cached;
	}
	ctx.font = `${weight} ${roundedSize}px ${fontFamily}`;
	if ("letterSpacing" in ctx) {
		ctx.letterSpacing = letterSpacingEm > 0 ? `${letterSpacingEm}em` : "0px";
	}
	const measured = ctx.measureText(text).width;
	if ("letterSpacing" in ctx) {
		ctx.letterSpacing = "0px";
	}
	widthCache.set(key, measured);
	return measured;
}

interface CandidateEntry {
	kind: "node" | "region";
	text: string;
	screenX: number;
	screenY: number;
	textWidth: number;
	fontSize: number;
	weight: 600 | 700;
	fontFamily: string;
	letterSpacingEm: number;
	opacity: number;
	color: string;
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

	if (!fontsReady || nodes.length === 0) return;

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
	const seen = new Set<number>();

	if (
		selectedIndex !== null &&
		selectedIndex >= 0 &&
		selectedIndex < nodes.length
	) {
		priorityIndices.push(selectedIndex);
		seen.add(selectedIndex);
	}
	if (
		hoveredIndex !== null &&
		hoveredIndex >= 0 &&
		hoveredIndex < nodes.length &&
		!seen.has(hoveredIndex)
	) {
		priorityIndices.push(hoveredIndex);
		seen.add(hoveredIndex);
	}
	if (focusEmphasis && focusEmphasis.neighborIndices.size > 0) {
		for (let i = 0; i < sortedIndicesByCount.length; i++) {
			const idx = sortedIndicesByCount[i] as number;
			if (focusEmphasis.neighborIndices.has(idx) && !seen.has(idx)) {
				priorityIndices.push(idx);
				seen.add(idx);
			}
		}
	}
	for (let i = 0; i < sortedIndicesByCount.length; i++) {
		const idx = sortedIndicesByCount[i] as number;
		if (!seen.has(idx)) {
			priorityIndices.push(idx);
			seen.add(idx);
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
		const isNeighbor = focusEmphasis?.neighborIndices.has(idx) ?? false;
		const isForced = isSelected || isHovered;

		const baseOpacity =
			isForced || isNeighbor ? 1 : computeLabelOpacity(viewDepth, activeFog);
		if (baseOpacity <= 0) continue;

		const paperMix = focusEmphasis?.nodePaperMix[idx] ?? 0;
		const opacity = paperMix > 0 ? baseOpacity * (1 - paperMix) : baseOpacity;
		if (opacity <= 0.02) continue;

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

		const typo = resolveNodeLabelTypography(node, isSelected);
		const weight = typo.fontWeight;
		const fontFamily = typo.fontFamily;
		const fontSize = Math.round(typo.fontSize * 10) / 10;
		const textWidth = measureLabelWidth(
			ctx,
			node.tagName,
			fontSize,
			weight,
			fontFamily,
		);

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
			kind: "node",
			text: node.tagName,
			screenX,
			screenY,
			textWidth,
			fontSize,
			weight,
			fontFamily,
			letterSpacingEm: 0,
			opacity,
			color: isSelected ? palette.accent : palette.labelInk,
			isSelected,
		});
	}

	const regionCandidates = buildRegionLabelCandidates(
		topicClusters,
		selectedIndex,
	);
	for (const region of regionCandidates) {
		const cx = region.centroid[0];
		const cy = region.centroid[1];
		const cz = region.centroid[2];

		const viewDepth =
			(cx - camX) * fwdX + (cy - camY) * fwdY + (cz - camZ) * fwdZ;
		if (viewDepth <= 0) continue;

		const fogOp = computeLabelOpacity(viewDepth, activeFog);
		if (fogOp <= 0) continue;

		_projVec.set(cx, cy, cz).project(cam);
		if (_projVec.z < -1 || _projVec.z > 1) continue;

		const screenX = (_projVec.x * 0.5 + 0.5) * cssW;
		const screenY = (-_projVec.y * 0.5 + 0.5) * cssH;
		if (screenX < 0 || screenX > cssW || screenY < 0 || screenY > cssH) {
			continue;
		}

		const textWidth = measureLabelWidth(
			ctx,
			region.text,
			region.fontSize,
			region.fontWeight,
			region.fontFamily,
			region.letterSpacingEm,
		);

		const boxW = textWidth + 8;
		const boxH = region.fontSize + 6;
		boxes.push({
			x: screenX - boxW * 0.5,
			y: screenY - boxH - 2,
			w: boxW,
			h: boxH,
			forced: false,
		});
		candidates.push({
			kind: "region",
			text: region.text,
			screenX,
			screenY,
			textWidth,
			fontSize: region.fontSize,
			weight: region.fontWeight,
			fontFamily: region.fontFamily,
			letterSpacingEm: region.letterSpacingEm,
			opacity: region.alpha * fogOp,
			color: region.ink,
			isSelected: false,
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
		ctx.font = `${entry.weight} ${entry.fontSize}px ${entry.fontFamily}`;
		if ("letterSpacing" in ctx) {
			ctx.letterSpacing =
				entry.letterSpacingEm > 0 ? `${entry.letterSpacingEm}em` : "0px";
		}
		ctx.strokeText(entry.text, entry.screenX, textY);
		ctx.fillStyle = entry.color;
		ctx.fillText(entry.text, entry.screenX, textY);
		if ("letterSpacing" in ctx) {
			ctx.letterSpacing = "0px";
		}

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
	void focusEmphasis;
	void topicClusters;
	invalidate();
});

onMount(() => {
	let active = true;
	if (typeof document !== "undefined" && document.fonts) {
		const fontPromises: Promise<unknown>[] = [];
		if (typeof document.fonts.load === "function") {
			fontPromises.push(
				document.fonts.load('600 20px "Playfair Display"'),
				document.fonts.load('700 20px "Playfair Display"'),
				document.fonts.load('600 12px "Source Sans 3"'),
				document.fonts.load('700 10px "Source Sans 3"'),
			);
		}
		if (document.fonts.ready) {
			fontPromises.push(document.fonts.ready);
		}
		Promise.all(fontPromises)
			.then(() => {
				if (active) {
					widthCache.clear();
					fontsReady = true;
					invalidate();
				}
			})
			.catch(() => {
				if (active) {
					fontsReady = true;
					invalidate();
				}
			});
	} else {
		fontsReady = true;
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
