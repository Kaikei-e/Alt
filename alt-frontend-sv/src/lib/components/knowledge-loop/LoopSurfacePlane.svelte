<script lang="ts">
/**
 * A single "plane" on the Knowledge Loop page. Alt-Paper renders planes
 * as section bands — a monospace plane label, a thin rule, then the
 * entries. No drop shadows; tonal hierarchy between foreground / mid
 * context / deep focus is carried by subtle background + saturation
 * shifts (ADR-000831 §12).
 */

type Plane = "foreground" | "mid-context" | "deep-focus";

let {
	plane = "foreground" as Plane,
	label,
	caption,
	children,
}: {
	plane?: Plane;
	label: string;
	caption?: string;
	children?: import("svelte").Snippet;
} = $props();
</script>

<section
	class="plane loop-plane"
	data-plane={plane}
	aria-label="Knowledge Loop {plane} plane"
>
	<header class="plane-head">
		<span class="plane-label">{label}</span>
		{#if caption}
			<span class="plane-caption">{caption}</span>
		{/if}
	</header>
	<div class="plane-rule" aria-hidden="true"></div>
	<div class="plane-body">
		{#if children}
			{@render children()}
		{/if}
	</div>
</section>

<style>
	.plane {
		display: grid;
		gap: 0.55rem;
		margin-bottom: 1.8rem;
	}
	.plane-head {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 1rem;
	}
	.plane-label {
		font-family: var(--font-body, "Source Sans 3", system-ui, sans-serif);
		font-size: 0.6rem;
		font-weight: 700;
		letter-spacing: 0.16em;
		text-transform: uppercase;
		color: var(--alt-charcoal, #1a1a1a);
	}
	.plane-caption {
		font-family: var(--font-mono, "IBM Plex Mono", ui-monospace, monospace);
		font-size: 0.65rem;
		color: var(--alt-ash, #999);
	}
	.plane-rule {
		height: 1px;
		background: var(--surface-border, #c8c8c8);
	}
	.plane-body {
		display: grid;
		gap: 0.7rem;
	}

	.plane[data-plane="mid-context"] {
		filter: saturate(0.94);
	}
	.plane[data-plane="deep-focus"] {
		filter: saturate(1.02);
	}

	@media (prefers-reduced-motion: reduce) {
		.plane[data-plane="mid-context"],
		.plane[data-plane="deep-focus"] {
			filter: none;
		}
	}
</style>
