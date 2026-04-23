<script lang="ts">
	/**
	 * Spatial surface plane container.
	 *
	 * Per ADR-000831 and docs/plan/knowledge-loop-canonical-contract.md §4.3 and §12:
	 *   - Depth lives on tiles, not on glyphs. This container provides a Z-layered frame;
	 *     text inside MUST stay flat.
	 *   - prefers-reduced-motion: reduce → Z movement / parallax are disabled and the tile
	 *     stack falls back to opacity / highlight fade. CSS handles this automatically.
	 */

	type Plane = "foreground" | "mid-context" | "deep-focus";

	let {
		plane = "foreground" as Plane,
		children,
	}: {
		plane?: Plane;
		children?: import("svelte").Snippet;
	} = $props();
</script>

<section class="surface-plane" data-plane={plane} aria-label={`Knowledge Loop ${plane} plane`}>
	{#if children}
		{@render children()}
	{/if}
</section>

<style>
	.surface-plane {
		display: grid;
		gap: var(--space-md, 1rem);
		padding: var(--space-md, 1rem);
		transform: translateZ(0); /* force compositor layer without parallax */
	}
	.surface-plane[data-plane="foreground"] {
		--plane-brightness: 1;
		--plane-saturation: 1;
	}
	.surface-plane[data-plane="mid-context"] {
		--plane-brightness: 0.92;
		--plane-saturation: 0.85;
		filter: brightness(var(--plane-brightness)) saturate(var(--plane-saturation));
	}
	.surface-plane[data-plane="deep-focus"] {
		--plane-brightness: 0.98;
		--plane-saturation: 1.05;
	}

	@media (prefers-reduced-motion: reduce) {
		.surface-plane {
			/* Reduced motion: no Z offsets, no parallax. Depth is signalled by subtle
			   brightness/saturation only. */
			filter: none;
		}
	}
</style>
