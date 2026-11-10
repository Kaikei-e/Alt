<script lang="ts">
import { T, useTask, useThrelte } from "@threlte/core";
import { OrbitControls } from "@threlte/extras";
import { untrack } from "svelte";
import * as THREE from "three";
import type { OrbitControls as ThreeOrbitControls } from "three/examples/jsm/controls/OrbitControls.js";
import { fogWindow, type SceneNode } from "./sceneModel.js";

interface Props {
	cloudRadius: number;
	selectedNode: SceneNode | null;
	autoRotate: boolean;
	reducedMotion: boolean;
	resetViewNonce: number;
}

let {
	cloudRadius,
	selectedNode,
	autoRotate,
	reducedMotion,
	resetViewNonce,
}: Props = $props();

const { scene, invalidate } = useThrelte();

const ANIMATION_DURATION = 0.6;

let cameraRef = $state<THREE.PerspectiveCamera | undefined>(undefined);
let controlsRef = $state<ThreeOrbitControls | undefined>(undefined);
let isAnimating = $state(false);

let animElapsed = 0;
const startCamPos = new THREE.Vector3();
const endCamPos = new THREE.Vector3();
const startTarget = new THREE.Vector3();
const endTarget = new THREE.Vector3();
const _viewDir = new THREE.Vector3();

function easeOutCubic(t: number): number {
	return 1 - (1 - t) ** 3;
}

function syncFog() {
	if (!cameraRef) return;
	const target = controlsRef?.target;
	const distance = target
		? cameraRef.position.distanceTo(target)
		: cameraRef.position.length();
	const { near, far } = fogWindow(distance, cloudRadius);
	if (scene.fog instanceof THREE.Fog) {
		scene.fog.near = near;
		scene.fog.far = far;
	}
}

$effect(() => {
	void cloudRadius;
	void cameraRef;
	void controlsRef;
	syncFog();
});

function transitionCamera(targetPos: THREE.Vector3, camPos: THREE.Vector3) {
	if (!cameraRef || !controlsRef) return;

	if (reducedMotion) {
		isAnimating = false;
		cameraRef.position.copy(camPos);
		controlsRef.target.copy(targetPos);
		controlsRef.update();
		syncFog();
		invalidate();
		return;
	}

	startCamPos.copy(cameraRef.position);
	startTarget.copy(controlsRef.target);
	endCamPos.copy(camPos);
	endTarget.copy(targetPos);
	animElapsed = 0;
	isAnimating = true;
	invalidate();
}

useTask(
	(delta) => {
		if (!cameraRef || !controlsRef) {
			isAnimating = false;
			return;
		}
		animElapsed += delta;
		const t = Math.min(1, animElapsed / ANIMATION_DURATION);
		const eased = easeOutCubic(t);

		cameraRef.position.lerpVectors(startCamPos, endCamPos, eased);
		controlsRef.target.lerpVectors(startTarget, endTarget, eased);
		controlsRef.update();
		syncFog();
		invalidate();

		if (t >= 1) {
			isAnimating = false;
		}
	},
	{
		running: () => isAnimating,
	},
);

let lastSelectedTag: string | null = null;

$effect(() => {
	const node = selectedNode;
	const nextName = node?.tagName ?? null;
	if (nextName === lastSelectedTag) return;
	lastSelectedTag = nextName;

	if (!node || !cameraRef || !controlsRef) return;

	const radius = untrack(() => cloudRadius);
	const target = new THREE.Vector3(
		node.position[0],
		node.position[1],
		node.position[2],
	);

	_viewDir.subVectors(cameraRef.position, controlsRef.target);
	if (_viewDir.lengthSq() < 1e-6) {
		_viewDir.set(0, 0, 1);
	} else {
		_viewDir.normalize();
	}

	const dist = Math.max(node.radius * 8, radius * 0.35);
	const camDest = target.clone().addScaledVector(_viewDir, dist);
	transitionCamera(target, camDest);
});

let lastResetNonce: number | undefined;

$effect(() => {
	const nonce = resetViewNonce;
	if (lastResetNonce === undefined) {
		lastResetNonce = nonce;
		return;
	}
	if (nonce === lastResetNonce) return;
	lastResetNonce = nonce;

	const radius = untrack(() => cloudRadius);
	const overviewTarget = new THREE.Vector3(0, 0, 0);
	const overviewCamPos = new THREE.Vector3(0, 0, radius * 1.5);
	transitionCamera(overviewTarget, overviewCamPos);
});
</script>

<T.PerspectiveCamera
	makeDefault
	bind:ref={cameraRef}
	position={[0, 0, cloudRadius * 1.5]}
	fov={50}
>
	<OrbitControls
		bind:ref={controlsRef}
		enableDamping
		dampingFactor={0.08}
		enablePan={false}
		{autoRotate}
		autoRotateSpeed={0.4}
		minDistance={cloudRadius * 0.3}
		maxDistance={cloudRadius * 5}
		onchange={syncFog}
	/>
</T.PerspectiveCamera>
