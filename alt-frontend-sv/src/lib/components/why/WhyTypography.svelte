<script lang="ts">
/**
 * WhyTypography — Newspaper-Style display primitive for a single WhyPayload.
 *
 * Renders the four pieces ADR-000908 §Δ4 made first-class:
 *   1. Kind banner — `WHY · KIND_NAME` in uppercase font-mono, sets the
 *      register before the reader engages with the narrative
 *   2. Confidence ladder — four ascending bars sepia-filled up to the
 *      current tier (SPECULATION → PATTERN → EVIDENCE → VERIFIED). Renders
 *      nothing when the tier is UNSPECIFIED so an absent signal does not
 *      read as "0/4 confidence"
 *   3. Narrative — the prose claim in serif (--font-display), the only
 *      element with rounded forms in the whole tile so the eye lands there
 *   4. Evidence / Counter-evidence — ref lists rendered as bordered
 *      monospace rails. Counter-evidence is folded behind an
 *      aria-expanded disclosure so the default state is the affirmative
 *      claim; objections live one tap away
 *
 * Reduced motion: the disclosure flips state without an animated max-height
 * when the OS reports prefers-reduced-motion. The rest of the tile is
 * static by design.
 */

export type WhyKindKey =
	| "source_why"
	| "pattern_why"
	| "recall_why"
	| "change_why"
	| "topic_affinity_why"
	| "tag_trending_why"
	| "unfinished_continue_why";

export type ConfidenceLadderTier =
	| "UNSPECIFIED"
	| "SPECULATION"
	| "PATTERN"
	| "EVIDENCE"
	| "VERIFIED";

export interface WhyEvidenceRef {
	refId: string;
	label?: string;
}

interface Props {
	kind?: WhyKindKey;
	text: string;
	confidenceLadder?: ConfidenceLadderTier;
	evidenceRefs?: WhyEvidenceRef[];
	counterEvidenceRefs?: WhyEvidenceRef[];
	whatWouldChangeMyMind?: string;
}

const {
	kind,
	text,
	confidenceLadder = "UNSPECIFIED",
	evidenceRefs = [],
	counterEvidenceRefs = [],
	whatWouldChangeMyMind,
}: Props = $props();

// Newspaper-style banner: "WHY · PATTERN" / "WHY · CHANGE" / etc.
// Every WhyKind has an explicit label so a new enum value can never silently
// fall back to the empty string — that's a regression the spec test guards
// against by enumerating the seven kinds.
const KIND_LABELS: Record<WhyKindKey, string> = {
	source_why: "SOURCE",
	pattern_why: "PATTERN",
	recall_why: "RECALL",
	change_why: "CHANGE",
	topic_affinity_why: "TOPIC AFFINITY",
	tag_trending_why: "TAG TRENDING",
	unfinished_continue_why: "UNFINISHED THREAD",
};

const kindLabel = $derived(kind ? KIND_LABELS[kind] : null);

// Map the qualitative ladder to a 0..4 step so the bar renderer is a pure
// fn of the proto enum. UNSPECIFIED resolves to 0 and the banner suppresses
// the indicator entirely.
const LADDER_STEPS: Record<ConfidenceLadderTier, number> = {
	UNSPECIFIED: 0,
	SPECULATION: 1,
	PATTERN: 2,
	EVIDENCE: 3,
	VERIFIED: 4,
};
const ladderStep = $derived(LADDER_STEPS[confidenceLadder]);
const showLadder = $derived(ladderStep > 0);

const ladderAriaLabel = $derived(
	showLadder
		? `Confidence: ${confidenceLadder.toLowerCase()} (${ladderStep} of 4)`
		: "Confidence not stated",
);

let counterExpanded = $state(false);
const hasCounterEvidence = $derived(counterEvidenceRefs.length > 0);
const counterCount = $derived(counterEvidenceRefs.length);

function toggleCounter() {
	counterExpanded = !counterExpanded;
}
</script>

<section class="why" aria-label="Why this surfaced">
	<header class="why-head">
		<span class="why-kind">
			<span class="why-kind-prefix" aria-hidden="true">WHY</span>
			<span class="why-kind-sep" aria-hidden="true">·</span>
			<span class="why-kind-label">{kindLabel ?? "REASON"}</span>
		</span>

		{#if showLadder}
			<span
				class="why-ladder"
				role="img"
				aria-label={ladderAriaLabel}
				data-tier={confidenceLadder}
			>
				{#each [1, 2, 3, 4] as step (step)}
					<span
						class="why-ladder-bar"
						class:why-ladder-bar--filled={step <= ladderStep}
						style:--step={step}
					></span>
				{/each}
			</span>
		{/if}
	</header>

	<p class="why-text">{text}</p>

	{#if evidenceRefs.length > 0}
		<section class="why-refs">
			<h3 class="why-refs-heading">
				<span class="why-refs-label">EVIDENCE</span>
				<span class="why-refs-count">{evidenceRefs.length}</span>
			</h3>
			<ol class="why-refs-list">
				{#each evidenceRefs as ref (ref.refId)}
					<li class="why-refs-item">
						<span class="why-refs-id">{ref.refId}</span>
						{#if ref.label}
							<span class="why-refs-sep" aria-hidden="true">—</span>
							<span class="why-refs-label-text">{ref.label}</span>
						{/if}
					</li>
				{/each}
			</ol>
		</section>
	{/if}

	{#if hasCounterEvidence}
		<section class="why-refs why-refs--counter">
			<h3 class="why-refs-heading">
				<button
					type="button"
					class="why-counter-toggle"
					onclick={toggleCounter}
					aria-expanded={counterExpanded}
					aria-controls="why-counter-body"
				>
					<span class="why-counter-caret" aria-hidden="true">
						{counterExpanded ? "▾" : "▸"}
					</span>
					<span class="why-refs-label">COUNTER-EVIDENCE</span>
					<span class="why-refs-count">{counterCount}</span>
				</button>
			</h3>
			<ol
				id="why-counter-body"
				class="why-refs-list why-refs-list--counter"
				class:why-refs-list--collapsed={!counterExpanded}
				aria-hidden={!counterExpanded}
			>
				{#each counterEvidenceRefs as ref (ref.refId)}
					<li class="why-refs-item">
						<span class="why-refs-id">{ref.refId}</span>
						{#if ref.label}
							<span class="why-refs-sep" aria-hidden="true">—</span>
							<span class="why-refs-label-text">{ref.label}</span>
						{/if}
					</li>
				{/each}
			</ol>
		</section>
	{/if}

	{#if whatWouldChangeMyMind}
		<section class="why-falsifier">
			<h3 class="why-refs-heading">
				<span class="why-refs-label">WHAT WOULD CHANGE MY MIND</span>
			</h3>
			<p class="why-falsifier-text">{whatWouldChangeMyMind}</p>
		</section>
	{/if}
</section>

<style>
.why {
	display: flex;
	flex-direction: column;
	gap: 0.55rem;
	color: var(--alt-charcoal, #1a1a1a);
}

.why-head {
	display: flex;
	align-items: baseline;
	justify-content: space-between;
	gap: 1rem;
	padding-bottom: 0.35rem;
	border-bottom: 1px solid var(--surface-border, #c8c8c8);
}

.why-kind {
	display: inline-flex;
	align-items: baseline;
	gap: 0.35rem;
	font-family: var(--font-mono, "IBM Plex Mono", ui-monospace, monospace);
	font-size: 0.68rem;
	letter-spacing: 0.08em;
	text-transform: uppercase;
	color: var(--alt-slate, #666);
}

.why-kind-prefix {
	color: var(--alt-ash, #999);
}

.why-kind-sep {
	color: var(--alt-ash, #999);
	font-weight: 400;
}

.why-kind-label {
	color: var(--alt-charcoal, #1a1a1a);
	font-weight: 600;
}

/* Confidence ladder: four ascending bars, sepia-fill up to the active tier. */
.why-ladder {
	display: inline-flex;
	align-items: flex-end;
	gap: 2px;
	height: 11px;
}

.why-ladder-bar {
	display: inline-block;
	width: 4px;
	height: calc(4px + (var(--step, 1) - 1) * 2.5px);
	background: transparent;
	border: 1px solid var(--alt-ash, #999);
	transition: background 120ms linear, border-color 120ms linear;
}

.why-ladder-bar--filled {
	border-color: var(--alt-charcoal, #1a1a1a);
	background: var(--alt-charcoal, #1a1a1a);
}

/* Sepia ladder: deepen the colour as the tier climbs, so VERIFIED reads
   darker than SPECULATION even at a glance. */
.why-ladder[data-tier="SPECULATION"] .why-ladder-bar--filled {
	background: #b89d6e;
	border-color: #b89d6e;
}
.why-ladder[data-tier="PATTERN"] .why-ladder-bar--filled {
	background: #8a6f47;
	border-color: #8a6f47;
}
.why-ladder[data-tier="EVIDENCE"] .why-ladder-bar--filled {
	background: #5d4a2c;
	border-color: #5d4a2c;
}
.why-ladder[data-tier="VERIFIED"] .why-ladder-bar--filled {
	background: var(--alt-charcoal, #1a1a1a);
	border-color: var(--alt-charcoal, #1a1a1a);
}

/* Why narrative — the one place serif appears, drawing the eye. */
.why-text {
	margin: 0;
	font-family: var(--font-display, "Playfair Display", Georgia, serif);
	font-size: 1rem;
	line-height: 1.45;
	color: var(--alt-charcoal, #1a1a1a);
}

/* Evidence / counter-evidence shared list typography. */
.why-refs {
	display: flex;
	flex-direction: column;
	gap: 0.3rem;
	padding-top: 0.25rem;
}

.why-refs--counter {
	border-top: 1px solid var(--surface-border, #c8c8c8);
	padding-top: 0.5rem;
}

.why-refs-heading {
	margin: 0;
	font-family: var(--font-mono, "IBM Plex Mono", ui-monospace, monospace);
	font-size: 0.62rem;
	letter-spacing: 0.1em;
	text-transform: uppercase;
	color: var(--alt-slate, #666);
	display: flex;
	align-items: center;
	gap: 0.4rem;
}

.why-refs-label {
	font-weight: 600;
	color: var(--alt-slate, #666);
}

.why-refs-count {
	color: var(--alt-ash, #999);
	font-weight: 500;
}

.why-counter-toggle {
	display: inline-flex;
	align-items: center;
	gap: 0.4rem;
	background: transparent;
	border: 0;
	padding: 0;
	margin: 0;
	font: inherit;
	letter-spacing: inherit;
	text-transform: inherit;
	color: inherit;
	cursor: pointer;
}

.why-counter-toggle:focus-visible {
	outline: 2px solid var(--alt-charcoal, #1a1a1a);
	outline-offset: 2px;
}

.why-counter-caret {
	display: inline-block;
	width: 0.7rem;
	color: var(--alt-slate, #666);
	font-size: 0.75rem;
}

.why-refs-list {
	list-style: none;
	margin: 0;
	padding: 0 0 0 0.5rem;
	border-left: 1px solid var(--surface-border, #c8c8c8);
	display: flex;
	flex-direction: column;
	gap: 0.2rem;
	overflow: hidden;
	transition: max-height 220ms ease, opacity 180ms ease;
}

.why-refs-list--counter {
	border-left-color: var(--alt-terracotta, #b85450);
	max-height: 8rem;
	opacity: 1;
}

.why-refs-list--collapsed {
	max-height: 0;
	opacity: 0;
}

.why-refs-item {
	font-family: var(--font-mono, "IBM Plex Mono", ui-monospace, monospace);
	font-size: 0.7rem;
	color: var(--alt-slate, #666);
	display: flex;
	gap: 0.3rem;
}

.why-refs-id {
	color: var(--alt-charcoal, #1a1a1a);
}

.why-refs-sep {
	color: var(--alt-ash, #999);
}

.why-refs-label-text {
	color: var(--alt-slate, #666);
}

.why-falsifier {
	display: flex;
	flex-direction: column;
	gap: 0.25rem;
	padding-top: 0.45rem;
	border-top: 1px dashed var(--surface-border, #c8c8c8);
}

.why-falsifier-text {
	margin: 0;
	font-family: var(--font-display, "Playfair Display", Georgia, serif);
	font-style: italic;
	font-size: 0.88rem;
	line-height: 1.4;
	color: var(--alt-slate, #666);
}

/* Reduced motion: drop the disclosure transition. */
@media (prefers-reduced-motion: reduce) {
	.why-refs-list {
		transition: none;
	}
	.why-ladder-bar {
		transition: none;
	}
}
</style>
