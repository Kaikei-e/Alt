<script lang="ts">
import { goto } from "$app/navigation";
import { page } from "$app/state";

/**
 * LensSelector — first-class top-bar control for the user's cognitive lens.
 * ADR-000909 §Δ5: lens is a declaration of "what mode am I in" (Research /
 * Browse / Decide / Recall) and Surface Planner v2 reads it as one of the
 * inputs to bucket weighting. Click flips the `?lens=` query param so the
 * +page.server.ts load picks up the new mode and emits the canonical
 * KnowledgeLoopLensModeSwitched event on transition.
 *
 * Newspaper Style baseline: en-dash separator, IBM Plex Mono, no shadow.
 * Reduced-motion: opacity-only transition (depth is on tiles, not glyphs).
 */

type LensId = "research" | "browse" | "decide" | "recall";

interface Props {
	activeLens: string;
}

const { activeLens }: Props = $props();

const OPTIONS: { id: LensId; label: string }[] = [
	{ id: "research", label: "Research" },
	{ id: "browse", label: "Browse" },
	{ id: "decide", label: "Decide" },
	{ id: "recall", label: "Recall" },
];

function isLensId(value: string): value is LensId {
	return (
		value === "research" ||
		value === "browse" ||
		value === "decide" ||
		value === "recall"
	);
}

const current: LensId = $derived(isLensId(activeLens) ? activeLens : "browse");

function selectLens(id: LensId) {
	if (id === current) return;
	const url = new URL(page.url);
	url.searchParams.set("lens", id);
	void goto(url.pathname + url.search, {
		invalidate: ["loop:data"],
		keepFocus: true,
		noScroll: true,
	});
}
</script>

<nav
	class="lens-selector"
	aria-label="Cognitive lens"
	data-testid="loop-lens-selector"
>
	<span class="lens-label" aria-hidden="true">Lens</span>
	<span class="lens-sep" aria-hidden="true">—</span>
	<ul class="lens-options" role="radiogroup">
		{#each OPTIONS as option, i (option.id)}
			{#if i > 0}
				<span class="lens-sep" aria-hidden="true">─</span>
			{/if}
			<li class="lens-option">
				<button
					type="button"
					role="radio"
					aria-checked={current === option.id}
					class:active={current === option.id}
					data-testid="loop-lens-option"
					data-lens-id={option.id}
					onclick={() => selectLens(option.id)}
				>
					{#if current === option.id}
						<span class="dot" aria-hidden="true">●</span>
					{/if}
					{option.label}
				</button>
			</li>
		{/each}
	</ul>
</nav>

<style>
.lens-selector {
	display: inline-flex;
	align-items: baseline;
	gap: 0.4rem;
	font-family: var(--font-mono, "IBM Plex Mono", ui-monospace, monospace);
	font-size: 0.72rem;
	letter-spacing: 0.06em;
	color: var(--alt-slate, #666);
}
.lens-label {
	font-size: 0.62rem;
	text-transform: uppercase;
	letter-spacing: 0.12em;
	color: var(--alt-ash, #999);
}
.lens-sep {
	color: var(--alt-tertiary, #808080);
}
.lens-options {
	list-style: none;
	display: inline-flex;
	align-items: baseline;
	gap: 0.4rem;
	padding: 0;
	margin: 0;
}
.lens-option {
	display: inline;
}
.lens-option button {
	appearance: none;
	background: transparent;
	border: none;
	padding: 0.1rem 0.3rem;
	font: inherit;
	color: var(--alt-slate, #666);
	cursor: pointer;
	transition: color 0.15s ease;
}
.lens-option button:hover {
	color: var(--alt-charcoal, #1a1a1a);
	text-decoration: underline;
	text-underline-offset: 3px;
}
.lens-option button:focus-visible {
	outline: 2px solid var(--alt-charcoal, #1a1a1a);
	outline-offset: 2px;
}
.lens-option button.active {
	color: var(--alt-charcoal, #1a1a1a);
	font-weight: 600;
}
.dot {
	margin-right: 0.2rem;
	color: var(--alt-primary, #2f4f4f);
}

@media (prefers-reduced-motion: reduce) {
	.lens-option button {
		transition: none;
	}
}
</style>
