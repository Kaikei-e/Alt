<script lang="ts">
import { T, useThrelte, useTask } from "@threlte/core";
import { OrbitControls, interactivity } from "@threlte/extras";
import { onDestroy } from "svelte";
import type { TagCloudItem } from "$lib/connect";
import TagAsteroid from "./TagAsteroid.svelte";
import StarBackground from "./StarBackground.svelte";
import PostProcessing from "./PostProcessing.svelte";
import * as THREE from "three";

interface Props {
	tags: TagCloudItem[];
	onTagSelect: (tagName: string) => void;
}

let { tags, onTagSelect }: Props = $props();

const { scene, camera } = useThrelte();

/**
 * Shared camera data computed once per frame instead of per-tag (300x reduction).
 * Plain object mutated in-place — children read it in their own useTask.
 * Parent useTask runs first (registered before children mount).
 */
export type CameraFrameData = {
	lookDirX: number;
	lookDirY: number;
	lookDirZ: number;
	camPosX: number;
	camPosY: number;
	camPosZ: number;
	depthOfCenter: number;
};

const cameraFrameData: CameraFrameData = {
	lookDirX: 0,
	lookDirY: 0,
	lookDirZ: 0,
	camPosX: 0,
	camPosY: 0,
	camPosZ: 0,
	depthOfCenter: 0,
};

const _lookDir = new THREE.Vector3();

useTask(() => {
	const cam = camera.current;
	cam.getWorldDirection(_lookDir);
	cameraFrameData.lookDirX = _lookDir.x;
	cameraFrameData.lookDirY = _lookDir.y;
	cameraFrameData.lookDirZ = _lookDir.z;
	cameraFrameData.camPosX = cam.position.x;
	cameraFrameData.camPosY = cam.position.y;
	cameraFrameData.camPosZ = cam.position.z;
	cameraFrameData.depthOfCenter = -(
		cam.position.x * _lookDir.x +
		cam.position.y * _lookDir.y +
		cam.position.z * _lookDir.z
	);
});

// Enable pointer events on 3D objects (required by Threlte v8)
interactivity();

// Dispose all scene materials BEFORE Canvas teardown (renderer.dispose()).
// Renderer.dispose() clears NodeManager's WeakMap, but Threlte's separate
// cleanup later dispatches material 'dispose' events that trigger
// NodeManager.delete on stale data → "Cannot read 'usedTimes' of undefined".
// By disposing materials here (while NodeManager data is still valid),
// RenderObject removes its event listeners, preventing the later crash.
onDestroy(() => {
	scene.traverse((obj) => {
		if ("material" in obj && obj.material) {
			const mat = obj.material as THREE.Material | THREE.Material[];
			if (Array.isArray(mat)) {
				for (const m of mat) m.dispose();
			} else {
				mat.dispose();
			}
		}
	});
});

// Compute color and emissive intensity from article count (5-level discrete scale)
function computeColor(
	articleCount: number,
	maxCount: number,
): { color: THREE.Color; emissiveIntensity: number } {
	const t = (articleCount / Math.max(maxCount, 1)) ** 0.5;

	if (t < 0.2) {
		return { color: new THREE.Color("#2a4a7a"), emissiveIntensity: 0.3 };
	}
	if (t < 0.4) {
		return { color: new THREE.Color("#2a8a9c"), emissiveIntensity: 0.45 };
	}
	if (t < 0.6) {
		return { color: new THREE.Color("#00d4ff"), emissiveIntensity: 0.6 };
	}
	if (t < 0.8) {
		return { color: new THREE.Color("#f0a030"), emissiveIntensity: 0.75 };
	}
	return { color: new THREE.Color("#ff4500"), emissiveIntensity: 0.9 };
}

// Compute radius from article count using power scale
function computeRadius(articleCount: number, maxCount: number): number {
	const minRadius = 0.3;
	const maxRadius = 3.0;
	const normalized = (articleCount / Math.max(maxCount, 1)) ** 0.4;
	return minRadius + normalized * (maxRadius - minRadius);
}

// Compute label font size proportional to radius
function computeLabelFontSize(articleCount: number, maxCount: number): number {
	const minSize = 11;
	const maxSize = 16;
	const normalized = (articleCount / Math.max(maxCount, 1)) ** 0.4;
	return minSize + normalized * (maxSize - minSize);
}

// Check if server provided positions
const hasServerPositions = $derived(
	tags.some((t) => t.positionX !== 0 || t.positionY !== 0 || t.positionZ !== 0),
);

// Fibonacci sphere fallback
function fibonacciSphere(
	count: number,
	radius: number,
): [number, number, number][] {
	const points: [number, number, number][] = [];
	const goldenAngle = Math.PI * (3 - Math.sqrt(5));
	for (let i = 0; i < count; i++) {
		const y = 1 - (i / Math.max(count - 1, 1)) * 2;
		const radiusAtY = Math.sqrt(1 - y * y);
		const theta = goldenAngle * i;
		const x = Math.cos(theta) * radiusAtY;
		const z = Math.sin(theta) * radiusAtY;
		points.push([x * radius, y * radius, z * radius]);
	}
	return points;
}

// Precompute tag data
const maxCount = $derived(
	tags.length > 0 ? Math.max(...tags.map((t) => t.articleCount)) : 1,
);

// Cloud radius from server positions or fallback
const cloudRadius = $derived(
	hasServerPositions
		? Math.max(
				60,
				Math.max(
					...tags.map((t) =>
						Math.sqrt(t.positionX ** 2 + t.positionY ** 2 + t.positionZ ** 2),
					),
				) * 1.2,
			)
		: Math.max(15, Math.sqrt(tags.length) * 3),
);

// Fallback positions (only computed when server positions not available)
const fallbackPositions = $derived(
	hasServerPositions ? null : fibonacciSphere(tags.length, cloudRadius),
);

function getPosition(
	tag: TagCloudItem,
	index: number,
): [number, number, number] {
	if (hasServerPositions) {
		return [tag.positionX, tag.positionY, tag.positionZ];
	}
	return fallbackPositions?.[index] ?? [0, 0, 0];
}

function handleTagSelect(tag: TagCloudItem) {
	onTagSelect(tag.tagName);
}
</script>

<T.PerspectiveCamera
	makeDefault
	position={[0, 0, cloudRadius * 1.5]}
	fov={60}
>
	<OrbitControls
		enableDamping
		dampingFactor={0.05}
		autoRotate
		autoRotateSpeed={0.3}
		minDistance={cloudRadius * 0.3}
		maxDistance={cloudRadius * 5}
		enablePan={false}
	/>
</T.PerspectiveCamera>

<!-- Lighting -->
<T.AmbientLight intensity={0.5} color="#6688cc" />
<T.PointLight position={[20, 20, 20]} intensity={1.2} color="#ffffff" />
<T.PointLight position={[-15, -10, 15]} intensity={0.6} color="#4488ff" />

<!-- Fog for depth (reduced density for better visibility) -->
<T.FogExp2 attach="fog" args={["#000000", 0.002]} />

<!-- Background -->
<T.Color attach="background" args={["#000000"]} />

<!-- Stars -->
<StarBackground />

<!-- Tag Asteroids -->
{#each tags as tag, i (tag.tagName)}
	{@const tagColor = computeColor(tag.articleCount, maxCount)}
	<TagAsteroid
		{tag}
		position={getPosition(tag, i)}
		radius={computeRadius(tag.articleCount, maxCount)}
		color={tagColor.color}
		emissiveIntensity={tagColor.emissiveIntensity}
		labelFontSize={computeLabelFontSize(tag.articleCount, maxCount)}
		onSelect={handleTagSelect}
		{cameraFrameData}
	/>
{/each}

<!-- Post processing -->
<PostProcessing />
