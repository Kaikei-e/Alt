<script lang="ts">
import { T } from "@threlte/core";
import { onDestroy } from "svelte";
import * as THREE from "three";

const STAR_COUNT = 3000;
const SPREAD = 200;

const positions = new Float32Array(STAR_COUNT * 3);
const sizes = new Float32Array(STAR_COUNT);

for (let i = 0; i < STAR_COUNT; i++) {
	const i3 = i * 3;
	// Distribute in a sphere
	const theta = Math.random() * Math.PI * 2;
	const phi = Math.acos(2 * Math.random() - 1);
	const r = SPREAD * Math.cbrt(Math.random());
	positions[i3] = r * Math.sin(phi) * Math.cos(theta);
	positions[i3 + 1] = r * Math.sin(phi) * Math.sin(theta);
	positions[i3 + 2] = r * Math.cos(phi);
	sizes[i] = Math.random() * 0.15 + 0.05;
}

const geometry = new THREE.BufferGeometry();
geometry.setAttribute("position", new THREE.BufferAttribute(positions, 3));
geometry.setAttribute("size", new THREE.BufferAttribute(sizes, 1));

onDestroy(() => {
	geometry.dispose();
});
</script>

<T.Points>
	<T is={geometry} />
	<T.PointsMaterial
		color="#ffffff"
		size={0.12}
		transparent
		opacity={0.8}
		sizeAttenuation
		depthWrite={false}
	/>
</T.Points>
