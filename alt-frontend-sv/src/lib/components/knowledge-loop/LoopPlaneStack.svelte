<script lang="ts">
/**
 * LoopPlaneStack — the Knowledge Loop plane navigator. Renders Now /
 * Continue / Changed / Review as a paper-stack: the active plane sits
 * forward at full ink density, the others are visible as receding
 * "headers peeking out" of the stack so the user always sees what is
 * waiting behind the front page.
 *
 * Movement is keyboard-first (`[` / `]` and `j` / `k`) and CTA-first;
 * no scrolljack. Mobile (≤ 768px) degrades to a tab strip with the
 * active plane stacked vertically — depth is replaced by ordinal flow.
 *
 * Reduced Motion fallback is contractual (canonical contract §12 / §3
 * Reduced Motion invariant): all `translateZ` and `scale` are stopped,
 * differentiation moves to ink density + saturation only.
 */

import { tick } from "svelte";
import type { Snippet } from "svelte";
import {
	STACK_ORDER,
	type LoopPlaneDescriptor,
	type PlaneKey,
} from "./loop-plane-keys";

let {
	planes,
	activeKey = $bindable("now"),
	plane,
	announceChanges = true,
}: {
	planes: LoopPlaneDescriptor[];
	activeKey?: PlaneKey;
	plane: Snippet<[PlaneKey]>;
	announceChanges?: boolean;
} = $props();

// Resolve the order of planes the user can walk through. Always render in
// the canonical Now → Review order so the stack reads front-to-back the
// same way regardless of whether some bucket is empty. Empty planes still
// participate as thin slices — the stack visually communicates "nothing in
// Continue right now" instead of silently disappearing.
const orderedPlanes = $derived(
	STACK_ORDER.map((key) => planes.find((p) => p.key === key)).filter(
		(p): p is LoopPlaneDescriptor => p !== undefined,
	),
);

const activeIndex = $derived(
	orderedPlanes.findIndex((p) => p.key === activeKey),
);

let liveRegion = $state("");

function setActive(next: PlaneKey) {
	if (next === activeKey) return;
	activeKey = next;
	if (announceChanges) {
		const desc = planes.find((p) => p.key === next);
		if (desc) {
			const noun = desc.count === 1 ? "item" : "items";
			liveRegion = `${desc.label} plane, ${desc.count} ${noun}`;
			void tick().then(() => {
				// Force aria-live polite announcement by clearing then resetting.
				// Avoids stale messages reading on the next change when the same
				// plane is selected twice (rare, but possible via direct CTA).
			});
		}
	}
}

function step(delta: number) {
	if (orderedPlanes.length === 0) return;
	const current = activeIndex < 0 ? 0 : activeIndex;
	const next = (current + delta + orderedPlanes.length) % orderedPlanes.length;
	setActive(orderedPlanes[next].key);
}

function onWindowKeydown(e: KeyboardEvent) {
	// Skip when the user is composing in a text field — keyboard nav must
	// never steal a typed `[`. Modifier keys disable the shortcut so browser
	// shortcuts (e.g. Cmd+[ / Cmd+]) keep working.
	if (e.metaKey || e.ctrlKey || e.altKey) return;
	const target = e.target as HTMLElement | null;
	if (target?.matches?.("input, textarea, [contenteditable=true]")) return;
	if (e.key === "]" || e.key === "j") {
		e.preventDefault();
		step(1);
	} else if (e.key === "[" || e.key === "k") {
		e.preventDefault();
		step(-1);
	}
}

function distance(idx: number): number {
	if (activeIndex < 0) return idx;
	return Math.abs(idx - activeIndex);
}

function depthState(idx: number): "active" | "near" | "mid" | "far" {
	const d = distance(idx);
	if (d === 0) return "active";
	if (d === 1) return "near";
	if (d === 2) return "mid";
	return "far";
}
</script>

<svelte:window onkeydown={onWindowKeydown} />

<section
	class="plane-stack"
	aria-roledescription="paper stack"
	aria-label="Knowledge Loop planes"
	data-active-key={activeKey}
>
	<nav class="plane-tabs" aria-label="Knowledge Loop plane selector">
		<ul class="tab-list">
			{#each orderedPlanes as p, i (p.key)}
				<li class="tab-item">
					<button
						type="button"
						class="tab"
						class:tab-active={p.key === activeKey}
						aria-pressed={p.key === activeKey}
						aria-controls="loop-plane-pane-{p.key}"
						data-testid="loop-plane-tab-{p.key}"
						data-depth-state={depthState(i)}
						onclick={() => setActive(p.key)}
					>
						<span class="tab-label">{p.label}</span>
						<span class="tab-count" aria-hidden="true">{p.count}</span>
						<span class="tab-sr" class:tab-sr-empty={p.count === 0}>
							{p.count === 1
								? `${p.count} item`
								: `${p.count} items`}
						</span>
					</button>
				</li>
			{/each}
		</ul>
		<p class="tabs-hint" aria-hidden="true">
			Use <kbd>[</kbd> / <kbd>]</kbd> to move between planes
		</p>
	</nav>

	<div
		class="stack-viewport"
		data-testid="loop-plane-stack-viewport"
	>
		{#each orderedPlanes as p, i (p.key)}
			{@const depth = depthState(i)}
			<article
				id="loop-plane-pane-{p.key}"
				class="pane"
				class:pane-active={p.key === activeKey}
				data-plane-key={p.key}
				data-depth-state={depth}
				data-ink-density={i}
				aria-hidden={p.key !== activeKey}
				inert={p.key !== activeKey ? true : null}
			>
				<header class="pane-head">
					<span class="pane-label">{p.label}</span>
					{#if p.caption}
						<span class="pane-caption">{p.caption}</span>
					{/if}
				</header>
				<div class="pane-rule" aria-hidden="true"></div>
				<div class="pane-body">
					{@render plane(p.key)}
				</div>
			</article>
		{/each}
	</div>

	<div class="sr-live" aria-live="polite" aria-atomic="true">{liveRegion}</div>
</section>

<style>
	.plane-stack {
		display: grid;
		gap: 1.1rem;
		margin-bottom: 2rem;
		/* Single perspective context for the whole stack so depth composes
		 * with the per-tile depth defined in loop-depth.css. preserve-3d on
		 * the descendants ensures the Z transforms compose rather than flatten. */
		perspective: 1100px;
		perspective-origin: 50% 35%;
	}

	.plane-tabs {
		display: grid;
		gap: 0.4rem;
	}
	.tab-list {
		list-style: none;
		padding: 0;
		margin: 0;
		display: flex;
		flex-wrap: wrap;
		gap: 0.55rem;
		align-items: stretch;
	}
	.tab-item {
		display: contents;
	}
	.tab {
		appearance: none;
		background: transparent;
		border: 1px solid var(--surface-border, #c8c8c8);
		padding: 0.35rem 0.7rem 0.4rem;
		display: inline-flex;
		align-items: baseline;
		gap: 0.45rem;
		cursor: pointer;
		font-family: var(--font-mono, "IBM Plex Mono", ui-monospace, monospace);
		color: var(--alt-slate, #666);
		transition:
			color 200ms ease,
			border-color 200ms ease,
			background-color 200ms ease;
	}
	.tab-label {
		font-size: 0.72rem;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		font-weight: 700;
	}
	.tab-count {
		font-size: 0.65rem;
		color: var(--alt-ash, #999);
	}
	.tab-sr {
		position: absolute;
		width: 1px;
		height: 1px;
		padding: 0;
		margin: -1px;
		overflow: hidden;
		clip: rect(0, 0, 0, 0);
		white-space: nowrap;
		border: 0;
	}
	.tab:hover {
		color: var(--alt-charcoal, #1a1a1a);
		border-color: var(--alt-charcoal, #1a1a1a);
	}
	.tab:focus-visible {
		outline: 2px solid var(--alt-terracotta, #b85450);
		outline-offset: 2px;
	}
	.tab-active {
		color: var(--alt-charcoal, #1a1a1a);
		border-color: var(--alt-charcoal, #1a1a1a);
		background: rgba(0, 0, 0, 0.04);
	}

	.tabs-hint {
		margin: 0;
		font-family: var(--font-mono, "IBM Plex Mono", ui-monospace, monospace);
		font-size: 0.6rem;
		color: var(--alt-ash, #999);
	}
	.tabs-hint kbd {
		font-family: inherit;
		border: 1px solid var(--surface-border, #c8c8c8);
		padding: 0 0.25rem;
		font-size: 0.58rem;
		background: var(--surface-2, #f5f4f1);
	}

	.stack-viewport {
		position: relative;
		display: grid;
		transform-style: preserve-3d;
		min-height: 12rem;
	}

	.pane {
		grid-area: 1 / 1; /* All panes occupy the same cell — depth handles ordering. */
		display: grid;
		gap: 0.55rem;
		padding: 0.85rem 0.95rem;
		background: var(--paper-bg, #fafaf7);
		border: 1px solid var(--surface-border, #c8c8c8);
		transform-style: preserve-3d;
		transform-origin: 50% 0%;
		transition:
			transform 320ms cubic-bezier(0.2, 0.8, 0.2, 1),
			opacity 240ms ease,
			filter 240ms ease,
			background-color 240ms ease;
		will-change: auto;
	}

	/* Ink density ramp — the back of the stack reads as aged kraft, the front
	 * as fresh newsprint. Color only shifts between #0f0f0f and #2a2a2a so the
	 * monochrome paper feel holds (Alt-Paper shared language, not Vaporwave). */
	.pane[data-ink-density="0"] {
		--ink-color: #0f0f0f;
		--paper-bg: #fafaf7;
	}
	.pane[data-ink-density="1"] {
		--ink-color: #1a1a1a;
		--paper-bg: #f4f3ee;
	}
	.pane[data-ink-density="2"] {
		--ink-color: #1f1f1f;
		--paper-bg: #efedea;
	}
	.pane[data-ink-density="3"] {
		--ink-color: #2a2a2a;
		--paper-bg: #e9e6e0;
	}
	.pane {
		color: var(--ink-color, #1a1a1a);
	}

	.pane[data-depth-state="active"] {
		transform: translateZ(0px) scale(1);
		opacity: 1;
		filter: none;
		z-index: 4;
		will-change: transform;
	}
	.pane[data-depth-state="near"] {
		transform: translateZ(-8px) scale(0.965);
		opacity: 0.86;
		filter: saturate(0.92);
		z-index: 3;
	}
	.pane[data-depth-state="mid"] {
		transform: translateZ(-16px) scale(0.93);
		opacity: 0.72;
		filter: saturate(0.78) blur(0.4px);
		z-index: 2;
	}
	.pane[data-depth-state="far"] {
		transform: translateZ(-26px) scale(0.9);
		opacity: 0.6;
		filter: saturate(0.7) blur(0.6px);
		z-index: 1;
	}

	/* Stage-aware depth overrides via CSS custom properties set on
	 * .loop-plane-root[data-stage="..."] in loop-depth.css.
	 *
	 * orient: Continue pane rises toward active depth so the user perceives
	 *   it as the relevant mid-context for the current Now entry.
	 * act:    Non-active panes recede further — the workspace commands own
	 *   the visual floor and the stack reads as background reference. */
	.pane[data-plane-key="continue"][data-depth-state="near"] {
		opacity: var(--loop-context-near-opacity, 0.86);
		filter: var(--loop-context-near-filter, saturate(0.92));
		transform: var(--loop-context-near-transform, translateZ(-8px) scale(0.965));
	}
	.pane[data-plane-key="continue"][data-depth-state="mid"] {
		opacity: var(--loop-context-mid-opacity, 0.72);
		filter: var(--loop-context-mid-filter, saturate(0.78) blur(0.4px));
		transform: var(--loop-context-mid-transform, translateZ(-16px) scale(0.93));
	}
	.pane:not([data-plane-key="continue"])[data-depth-state="near"] {
		opacity: var(--loop-bg-near-opacity, 0.86);
	}
	.pane:not([data-plane-key="continue"])[data-depth-state="mid"] {
		opacity: var(--loop-bg-mid-opacity, 0.72);
	}
	.pane:not([data-plane-key="continue"])[data-depth-state="far"] {
		opacity: var(--loop-bg-far-opacity, 0.60);
	}

	/* Body content collapses on inactive panes so only the header peeks out of
	 * the stack — preserves the "newspaper edge sticking out" feel without
	 * starving the active plane of vertical space. */
	.pane:not(.pane-active) .pane-body {
		max-height: 0;
		overflow: hidden;
		opacity: 0;
		transition:
			max-height 240ms ease,
			opacity 200ms ease;
	}
	.pane.pane-active .pane-body {
		max-height: none;
		opacity: 1;
	}

	.pane-head {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 1rem;
	}
	.pane-label {
		font-family: var(--font-body, "Source Sans 3", system-ui, sans-serif);
		font-size: 0.62rem;
		font-weight: 700;
		letter-spacing: 0.16em;
		text-transform: uppercase;
		color: var(--ink-color, #1a1a1a);
	}
	.pane-caption {
		font-family: var(--font-mono, "IBM Plex Mono", ui-monospace, monospace);
		font-size: 0.65rem;
		color: var(--alt-ash, #999);
	}
	.pane-rule {
		height: 1px;
		background: var(--ink-color, #1a1a1a);
		opacity: 0.45;
	}
	.pane-body {
		display: grid;
		gap: 0.7rem;
	}

	.sr-live {
		position: absolute;
		width: 1px;
		height: 1px;
		padding: 0;
		margin: -1px;
		overflow: hidden;
		clip: rect(0, 0, 0, 0);
		white-space: nowrap;
		border: 0;
	}

	@media (prefers-reduced-motion: reduce) {
		.pane,
		.pane[data-depth-state="active"],
		.pane[data-depth-state="near"],
		.pane[data-depth-state="mid"],
		.pane[data-depth-state="far"] {
			transform: none;
			transition:
				opacity 120ms ease,
				filter 120ms ease;
			will-change: auto;
			filter: none;
		}
		.pane[data-depth-state="near"] {
			opacity: 0.85;
			filter: saturate(0.85);
		}
		.pane[data-depth-state="mid"] {
			opacity: 0.7;
			filter: saturate(0.7);
		}
		.pane[data-depth-state="far"] {
			opacity: 0.55;
			filter: saturate(0.55);
		}
	}

	@media (max-width: 768px) {
		/* Mobile / touch fallback. The 3D stack collapses into a flat tab
		 * strip with the active plane vertically expanded. Inactive panes
		 * disappear entirely — depth is replaced by ordinal flow.
		 * gyroscope-driven parallax is intentionally NOT implemented (battery
		 * + a11y). */
		.stack-viewport {
			perspective: none;
			transform-style: flat;
		}
		.pane,
		.pane[data-depth-state="active"],
		.pane[data-depth-state="near"],
		.pane[data-depth-state="mid"],
		.pane[data-depth-state="far"] {
			transform: none;
			transition:
				opacity 120ms ease;
			filter: none;
			will-change: auto;
		}
		.pane:not(.pane-active) {
			display: none;
		}
		.pane.pane-active {
			grid-area: auto;
		}
	}
</style>
