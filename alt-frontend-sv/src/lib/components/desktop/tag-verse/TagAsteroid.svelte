<script lang="ts">
import { T, useTask } from "@threlte/core";
import { HTML } from "@threlte/extras";
import { onDestroy } from "svelte";
import * as THREE from "three";
import type { TagCloudItem } from "$lib/connect";
import type { CameraFrameData } from "./SceneContent.svelte";

interface Props {
	tag: TagCloudItem;
	position: [number, number, number];
	radius: number;
	color: THREE.Color;
	emissiveIntensity: number;
	labelFontSize: number;
	onSelect: (tag: TagCloudItem) => void;
	cameraFrameData: CameraFrameData;
}

let { tag, position, radius, color, emissiveIntensity, labelFontSize, onSelect, cameraFrameData }: Props =
	$props();

let hovered = $state(false);
let labelVisible = $state(true);
let meshRef = $state<THREE.Mesh | undefined>(undefined);

onDestroy(() => {
	if (hovered) {
		document.body.style.cursor = "auto";
	}
});

// Slow self-rotation
const rotationSpeed = (Math.random() - 0.5) * 0.3;
const rotationAxis = new THREE.Vector3(
	Math.random() - 0.5,
	Math.random() - 0.5,
	Math.random() - 0.5,
).normalize();

useTask((delta) => {
	if (meshRef) {
		meshRef.rotateOnAxis(rotationAxis, rotationSpeed * delta);
	}

	// Far-side label occlusion using pre-computed camera data from parent
	const toX = position[0] - cameraFrameData.camPosX;
	const toY = position[1] - cameraFrameData.camPosY;
	const toZ = position[2] - cameraFrameData.camPosZ;
	const depthOfPlanet =
		toX * cameraFrameData.lookDirX +
		toY * cameraFrameData.lookDirY +
		toZ * cameraFrameData.lookDirZ;

	// Show label only if planet is in front half (with 15% margin past center)
	labelVisible = depthOfPlanet <= cameraFrameData.depthOfCenter * 1.15;
});

function handlePointerEnter() {
	hovered = true;
	document.body.style.cursor = "pointer";
}

function handlePointerLeave() {
	hovered = false;
	document.body.style.cursor = "auto";
}

function handleClick() {
	onSelect(tag);
}

const scale = $derived(hovered ? 1.15 : 1.0);
</script>

<T.Group {position}>
	<T.Mesh
		bind:ref={meshRef}
		scale={[scale * radius, scale * radius, scale * radius]}
		onpointerenter={handlePointerEnter}
		onpointerleave={handlePointerLeave}
		onclick={handleClick}
	>
		<T.SphereGeometry args={[1, 16, 16]} />
		<T.MeshStandardMaterial
			color={color}
			metalness={0.3}
			roughness={0.7}
			emissive={color}
			{emissiveIntensity}
		/>
	</T.Mesh>

	<!-- Always-visible tag name label (hidden when on far side) -->
	{#if labelVisible}
		<HTML
			position={[0, radius * 1.3, 0]}
			center
			pointerEvents="none"
			sprite
			zIndexRange={[30, 0]}
			occlude={true}
		>
			<span
				class="tag-label"
				style:font-size="{labelFontSize}px"
			>
				{tag.tagName}
			</span>
		</HTML>
	{/if}

	<!-- Hover tooltip with details -->
	{#if hovered && labelVisible}
		<HTML
			position={[0, radius * 1.3 + 2.0, 0]}
			center
			pointerEvents="none"
			sprite
			zIndexRange={[30, 0]}
			occlude={true}
		>
			<div class="tag-tooltip">
				<strong>{tag.tagName}</strong>
				<span class="article-count">{tag.articleCount} articles</span>
			</div>
		</HTML>
	{/if}
</T.Group>

<style>
	.tag-label {
		color: #ffffff;
		font-family: system-ui, -apple-system, sans-serif;
		font-weight: 500;
		white-space: nowrap;
		text-shadow:
			0 0 4px rgba(0, 0, 0, 0.9),
			0 0 8px rgba(0, 0, 0, 0.7),
			0 1px 2px rgba(0, 0, 0, 0.8);
		pointer-events: none;
		user-select: none;
	}

	.tag-tooltip {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 2px;
		padding: 6px 12px;
		background: rgba(10, 10, 30, 0.9);
		border: 1px solid rgba(0, 212, 255, 0.4);
		border-radius: 6px;
		backdrop-filter: blur(4px);
		white-space: nowrap;
		pointer-events: none;
		user-select: none;
	}

	.tag-tooltip strong {
		color: #ffffff;
		font-family: system-ui, -apple-system, sans-serif;
		font-size: 15px;
		font-weight: 600;
	}

	.tag-tooltip .article-count {
		color: #00d4ff;
		font-family: system-ui, -apple-system, sans-serif;
		font-size: 12px;
	}
</style>
