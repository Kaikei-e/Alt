<script lang="ts">
import { goto } from "$app/navigation";
import type {
	ConfidenceLadderName,
	DecisionIntentName,
	DecisionOptionData,
	KnowledgeLoopEntryData,
	LoopStageName,
} from "$lib/connect/knowledge_loop";
import type { TransitionMetadata } from "$lib/hooks/useKnowledgeLoop.svelte";
import WhyTypography, {
	type ConfidenceLadderTier,
} from "$lib/components/why/WhyTypography.svelte";
import {
	buildAskTransitionMetadata,
	buildRecapTransitionMetadata,
	buildTransitionMetadata,
} from "./transition-metadata";

// Convert the lowercase wire form of the confidence ladder ("speculation"
// etc.) to the uppercase tier the typography primitive consumes. Missing /
// undefined collapses to "UNSPECIFIED" so the indicator stays hidden.
function ladderTierFromName(
	name: ConfidenceLadderName | undefined,
): ConfidenceLadderTier {
	switch (name) {
		case "speculation":
			return "SPECULATION";
		case "pattern":
			return "PATTERN";
		case "evidence":
			return "EVIDENCE";
		case "verified":
			return "VERIFIED";
		default:
			return "UNSPECIFIED";
	}
}

type TransitionTrigger =
	| "user_tap"
	| "dwell"
	| "keyboard"
	| "programmatic"
	| "defer";

type Props = {
	entry: KnowledgeLoopEntryData;
	stagger?: number;
	onTransition?: (
		entryKey: string,
		toStage: LoopStageName,
		trigger?: TransitionTrigger,
		metadata?: TransitionMetadata,
		options?: { optimistic?: boolean },
	) => Promise<unknown> | unknown;
	onDismiss?: (
		entryKey: string,
		metadata?: TransitionMetadata,
	) => Promise<unknown> | unknown;
	onAsk?: (
		entry: KnowledgeLoopEntryData,
	) => Promise<{ conversationId?: string } | void | unknown> | unknown;
	onOpen?: (
		entry: KnowledgeLoopEntryData,
		href: string,
	) => Promise<unknown> | unknown;
	// ADR-000914: "I got this" graduation CTA. Fires the canonical
	// TRANSITION_TRIGGER_INTERNALIZE same-stage transition; the projector
	// flips dismiss_state to INTERNALIZED so the entry leaves foreground /
	// Continue / Now reads without touching freshness_at or why_text.
	onInternalize?: (entry: KnowledgeLoopEntryData) => Promise<unknown> | unknown;
	canTransition?: (from: LoopStageName, to: LoopStageName) => boolean;
	isInFlight?: (entryKey: string) => boolean;
	resolveSourceUrl?: (entry: KnowledgeLoopEntryData) => string | null;
};

let {
	entry,
	stagger = 0,
	onTransition,
	onDismiss,
	onAsk,
	onOpen,
	onInternalize,
	canTransition,
	isInFlight,
	resolveSourceUrl,
}: Props = $props();

/**
 * Alt-Paper responsibility-driven expression for Knowledge Loop:
 * each entry is a "ledger row" — a ruled record of a deliberative step in
 * the OODA cycle. We borrow Alt-Paper idioms (thin rules, uppercase labels,
 * serif why-text, monospace metadata, 3px status stripe) but shape them
 * around *process* rather than *publication* or *consultation*.
 *
 * Depth per ADR-000831 §12: Alt-Paper prohibits shadows, so render_depth_hint
 * collapses to a subtle saturate+brightness filter plus a rule-weight bump,
 * never elevation. Reduced-motion users get the weight bump alone.
 */

const effectiveStage = $derived(entry.currentEntryStage ?? entry.proposedStage);

const stageLabel = $derived(
	(
		{
			observe: "Observe",
			orient: "Orient",
			decide: "Decide",
			act: "Act",
		} as const
	)[effectiveStage],
);

const priorityLabel = $derived(
	(
		{
			critical: "Critical",
			continuing: "Continuing",
			confirm: "Confirm",
			reference: "Reference",
		} as const
	)[entry.loopPriority],
);

const ariaDescription = $derived(`Priority: ${priorityLabel}`);

const relFresh = $derived.by(() => {
	if (!entry.freshnessAt) return "—";
	const diff = Date.now() - new Date(entry.freshnessAt).getTime();
	if (diff < 60_000) return "just now";
	if (diff < 3_600_000) return `${Math.floor(diff / 60_000)}m ago`;
	if (diff < 86_400_000) return `${Math.floor(diff / 3_600_000)}h ago`;
	return `${Math.floor(diff / 86_400_000)}d ago`;
});

// PR-L3 wires the `ask` intent to the Augur handshake; if no onAsk callback
// is injected the CTA is still silently filtered so the tile stays harmless
// in environments that have not adopted the handshake.
const visibleOptions = $derived(
	entry.decisionOptions.filter(
		(o: DecisionOptionData) => o.intent !== "ask" || Boolean(onAsk),
	),
);

let expanded = $state(false);
let dismissing = $state(false);

const inFlight = $derived(isInFlight ? isInFlight(entry.entryKey) : false);

/**
 * Recap first-class CTA (Stream 2C). The projector seeds
 * `entry.actTargets` with `{targetType: "recap", route: "/recap/topic/<id>"}`
 * when its Surface Planner v2 inputs resolved a matching
 * RecapTopicSnapshotted event. We only render the CTA when the route is a
 * server-relative path with no scheme separator — defense in depth against
 * a regressed upstream that could otherwise smuggle a `javascript:` URL.
 */
const recapTarget = $derived(
	entry.actTargets.find((t) => t.targetType === "recap"),
);
const recapRoute = $derived.by(() => {
	const r = recapTarget?.route;
	if (!r) return null;
	// Must be a single-leading-slash absolute path. `//evil.com/x` is a
	// protocol-relative URL the browser resolves to https://evil.com/x —
	// open-redirect (CWE-601). `/\evil.com/x` is the same after browser
	// backslash-normalisation, so reject that shape too. The colon check
	// catches `javascript:` / `data:` / explicit `:port` smuggling.
	if (!r.startsWith("/")) return null;
	if (r.startsWith("//")) return null;
	if (r.startsWith("/\\")) return null;
	if (r.includes(":")) return null;
	return r;
});

function ctaToStage(intent: DecisionIntentName): LoopStageName | null {
	switch (intent) {
		// observe → orient: the user is opening the entry's mid-context plane.
		case "revisit":
			return "orient";
		// orient → decide: the user is comparing options before committing.
		case "compare":
			return "decide";
		// decide → act: open / save are act-stage commits.
		case "open":
		case "save":
			return "act";
		// `ask` and `snooze` do not drive a stage transition directly; ask
		// hands off to Augur, snooze defers locally. Returning null here keeps
		// them enabled regardless of the proposed_stage's transition allowlist.
		default:
			return null;
	}
}

function intentLabel(intent: DecisionIntentName): string {
	switch (intent) {
		case "open":
			return "Open";
		case "ask":
			return "Ask";
		case "save":
			return "Save";
		case "snooze":
			return "Snooze";
		case "compare":
			return "Compare";
		case "revisit":
			return "Revisit";
		default:
			return intent;
	}
}

function isAllowed(to: LoopStageName): boolean {
	if (!canTransition) return true;
	return canTransition(effectiveStage, to);
}

function sourceUrl(): string | null {
	if (resolveSourceUrl) return resolveSourceUrl(entry);
	const article = entry.actTargets.find((t) => t.targetType === "article");
	if (article?.route) return article.route;
	return null;
}

function transitionMetadata(option: DecisionOptionData): TransitionMetadata {
	return buildTransitionMetadata(entry, option);
}

function openHref(href: string) {
	if (onOpen) {
		void onOpen(entry, href);
		return;
	}
	window.open(href, "_blank", "noopener,noreferrer");
}

function toggleExpanded() {
	const wasCollapsed = !expanded;
	expanded = !expanded;
	// Auto-OODA suppression (plan: Knowledge Loop 体験回復 — Pillar 1):
	// the explicit user tap that expands the tile is also the gesture that
	// advances Observe → Orient. The previous IntersectionObserver-driven
	// dwell path is removed; this is the only legitimate cross-stage entry
	// for an observe-stage entry. Optimistic — onTransition is fire-and-
	// forget; the hook's applyLocalStage flips data-stage immediately.
	if (
		wasCollapsed &&
		effectiveStage === "observe" &&
		onTransition &&
		isAllowed("orient")
	) {
		// Optimistic: flip the local stage immediately so the user sees the
		// gesture they just performed reflected in the workspace before the
		// BFF reply lands. The hook reverts on non-accepted results.
		void onTransition(entry.entryKey, "orient", "user_tap", undefined, {
			optimistic: true,
		});
	}
}

function onTriggerKey(event: KeyboardEvent) {
	if (event.key === "Enter" || event.key === " ") {
		event.preventDefault();
		toggleExpanded();
	}
}

async function handleCta(option: DecisionOptionData) {
	if (option.intent === "ask") {
		if (!onAsk) return;
		// Phase 2 semantic loop: only emit the Loop transition after the Augur
		// handshake succeeds (per plan §3 — session creation failure must not
		// produce a phantom acted=ask event).
		const result = (await onAsk(entry)) as
			| { conversationId?: string }
			| void
			| undefined;
		const conversationId =
			result && typeof result === "object" && "conversationId" in result
				? result.conversationId
				: undefined;
		if (conversationId && onTransition) {
			const askMeta = buildAskTransitionMetadata(entry, conversationId);
			void onTransition(entry.entryKey, "decide", "user_tap", askMeta);
		}
		return;
	}
	// Snooze maps to the canonical defer transition (KnowledgeLoopDeferred,
	// contract §8.2). The hook's `dismiss` already emits a same-stage
	// `defer` transition; we extend it with Phase 2 semantic metadata so
	// the projector can record `acted_intent=snooze` / `target_type=entry`
	// in continue_context.recent_action_labels.
	if (option.intent === "snooze") {
		if (!onDismiss) return;
		const snoozeMeta = transitionMetadata(option);
		dismissing = true;
		await onDismiss(entry.entryKey, snoozeMeta);
		return;
	}

	const to = ctaToStage(option.intent);
	if (!to || !onTransition) return;
	if (!isAllowed(to)) return;
	const metadata = transitionMetadata(option);
	if (option.intent === "open") {
		const url = sourceUrl();
		if (url) {
			openHref(url);
		}
		void onTransition(entry.entryKey, to, "user_tap", metadata);
		return;
	}
	await onTransition(entry.entryKey, to, "user_tap", metadata);
}

function nextStageFor(stage: LoopStageName): LoopStageName {
	switch (stage) {
		case "observe":
			return "orient";
		case "orient":
			return "decide";
		case "decide":
			return "act";
		case "act":
			return "observe";
	}
}

async function handleOpenRecap(event: Event) {
	event.preventDefault();
	event.stopPropagation();
	if (!recapRoute || !recapTarget) return;
	if (onTransition) {
		const meta = buildRecapTransitionMetadata(entry, recapTarget);
		// Open Recap advances one OODA step (per canonical contract §7). The
		// fixed `act` target before this fix violated `orient → act` which the
		// allow-list rejects. Picking the natural next stage keeps the
		// transition legal regardless of where the entry is in the loop.
		const to = nextStageFor(effectiveStage);
		// Fire transition before navigation so Loop sees the deliberate intent.
		// We deliberately do not await: warn-and-continue keeps Open Recap
		// responsive even if the BFF is degraded (UI failure here is silent).
		void onTransition(entry.entryKey, to, "user_tap", meta);
	}
	await goto(recapRoute);
}

async function handleDismiss() {
	if (!onDismiss) return;
	dismissing = true;
	await onDismiss(entry.entryKey);
}
</script>

<!-- svelte-ignore a11y_no_noninteractive_element_to_interactive_role -->
<article
	class="entry depth-{entry.renderDepthHint}"
	class:expanded
	class:exit-pending={dismissing}
	data-role="loop-entry"
	data-testid="loop-entry-tile"
	data-entry-key={entry.entryKey}
	data-priority={entry.loopPriority}
	data-stage={effectiveStage}
	data-planner-version={entry.surfacePlannerVersion ?? "unspecified"}
	aria-label={ariaDescription}
	aria-expanded={expanded}
	role="button"
	tabindex="0"
	style="--stagger: {stagger}"
	onclick={toggleExpanded}
	onkeydown={onTriggerKey}
>
	<span class="entry-stripe" aria-hidden="true"></span>
	<div class="entry-body">
		<header class="entry-head">
			<span class="stage-label">{stageLabel}</span>
			<span class="priority-label">{priorityLabel}</span>
		</header>
		<WhyTypography
			kind={entry.whyPrimary.kind}
			text={entry.whyPrimary.text}
			confidenceLadder={ladderTierFromName(entry.whyPrimary.confidenceLadder)}
			evidenceRefs={entry.whyPrimary.evidenceRefs}
			counterEvidenceRefs={entry.whyPrimary.counterEvidenceRefs}
			whatWouldChangeMyMind={entry.whyPrimary.whatWouldChangeMyMind}
		/>
		{#if expanded}
			<section class="expand">
				{#if entry.changeSummary?.summary}
					<div class="expand-block">
						<h3 class="expand-heading">What changed</h3>
						<p class="expand-text">{entry.changeSummary.summary}</p>
					</div>
				{/if}
				{#if entry.continueContext?.summary}
					<div class="expand-block">
						<h3 class="expand-heading">Continue</h3>
						<p class="expand-text">{entry.continueContext.summary}</p>
					</div>
				{/if}
				<div class="cta-row">
					{#each visibleOptions as option (option.actionId)}
						{@const toStage = ctaToStage(option.intent)}
						{@const isAskCta = option.intent === "ask"}
						{@const isSnoozeCta = option.intent === "snooze"}
						{@const disabled = isAskCta
							? inFlight || !onAsk
							: isSnoozeCta
								? inFlight || !onDismiss || dismissing
								: toStage
									? inFlight || !isAllowed(toStage)
									: true}
						<button
							type="button"
							class="cta cta--{option.intent}"
							title={disabled && !inFlight
								? "Not available from this stage."
								: undefined}
							{disabled}
							onclick={(event) => {
								event.stopPropagation();
								void handleCta(option);
							}}
						>
							{option.label ?? intentLabel(option.intent)}
						</button>
					{/each}
					{#if recapRoute}
						<button
							type="button"
							class="cta cta--recap"
							onclick={(event) => void handleOpenRecap(event)}
						>
							Open Recap
						</button>
					{/if}
					<button
						type="button"
						class="cta cta--dismiss"
						disabled={dismissing}
						onclick={(event) => {
							event.stopPropagation();
							void handleDismiss();
						}}
					>
						Dismiss
					</button>
					{#if onInternalize}
						<button
							type="button"
							class="cta cta--internalize"
							aria-label="Mark as internalized; remove from Loop"
							disabled={dismissing}
							onclick={(event) => {
								event.stopPropagation();
								void onInternalize(entry);
							}}
						>
							I got this
						</button>
					{/if}
				</div>
			</section>
		{/if}
		<footer class="entry-foot">
			<span class="foot-cell">rev {entry.projectionRevision}</span>
			<span class="foot-cell foot-cell--freshness">{relFresh}</span>
			{#if entry.supersededByEntryKey}
				<span class="foot-cell foot-cell--super">Superseded</span>
			{/if}
		</footer>
	</div>
</article>

<style>
	.entry {
		display: grid;
		grid-template-columns: 3px 1fr;
		gap: 0;
		border: 1px solid var(--surface-border, #c8c8c8);
		background: var(--surface-bg, #faf9f7);
		animation: entry-in 0.3s ease forwards;
		animation-delay: calc(var(--stagger, 0) * 40ms);
		opacity: 0;
		cursor: pointer;
		text-align: left;
		/* No `max-height` clamp here. Pre-fix the tile collapsed `max-height: 0`
		 * during a `.dismissing` keyframe, which combined with the fetch-storm
		 * starving the main thread caused content overflow into the next grid
		 * row — visible as the OODA cards stacking onto each other. The exit
		 * animation now lives on the parent `#each` (`out:loopRecede` +
		 * `animate:flip`) and removes the row from the DOM cleanly. */
		transition:
			filter 180ms ease,
			border-color 180ms ease,
			transform 240ms cubic-bezier(0.2, 0, 0.1, 1);
	}
	.entry:focus-visible {
		outline: 2px solid var(--alt-charcoal, #1a1a1a);
		outline-offset: 2px;
	}
	.entry.exit-pending {
		/* Disable pointer events the moment Dismiss is clicked so the user can't
		 * fire a second action mid-exit. Visual fade is owned by `out:loopRecede`
		 * on the parent row. */
		pointer-events: none;
	}
	@keyframes entry-in {
		to {
			opacity: 1;
		}
	}

	.entry-stripe {
		background: var(--alt-slate, #666);
		align-self: stretch;
	}
	.entry[data-priority="critical"] .entry-stripe {
		background: var(--alt-terracotta, #b85450);
	}
	.entry[data-priority="continuing"] .entry-stripe {
		background: var(--alt-sand, #d4a574);
	}
	.entry[data-priority="confirm"] .entry-stripe {
		background: var(--alt-primary, #2f4f4f);
	}
	.entry[data-priority="reference"] .entry-stripe {
		background: var(--alt-ash, #999);
	}

	.entry-body {
		padding: 0.9rem 1.1rem 0.85rem;
	}

	.entry-head {
		display: flex;
		justify-content: space-between;
		align-items: baseline;
		gap: 0.75rem;
		margin-bottom: 0.55rem;
	}
	.stage-label,
	.priority-label {
		font-family: var(--font-body, "Source Sans 3", system-ui, sans-serif);
		font-size: 0.65rem;
		font-weight: 700;
		letter-spacing: 0.12em;
		text-transform: uppercase;
	}
	.stage-label {
		color: var(--alt-charcoal, #1a1a1a);
	}
	.priority-label {
		color: var(--alt-ash, #999);
	}
	.entry[data-priority="critical"] .priority-label {
		color: var(--alt-terracotta, #b85450);
	}

	.why-text {
		font-family: var(--font-body, "Source Sans 3", system-ui, sans-serif);
		font-size: 0.95rem;
		line-height: 1.65;
		color: var(--alt-charcoal, #1a1a1a);
		margin: 0;
		max-width: 65ch;
	}

	.evidence {
		margin-top: 0.7rem;
		padding-top: 0.55rem;
		border-top: 1px solid var(--surface-border, #c8c8c8);
	}
	.evidence-heading {
		font-family: var(--font-body, "Source Sans 3", system-ui, sans-serif);
		font-size: 0.6rem;
		font-weight: 700;
		letter-spacing: 0.12em;
		text-transform: uppercase;
		color: var(--alt-ash, #999);
		margin: 0 0 0.35rem;
	}
	.evidence-list {
		list-style: decimal;
		padding-left: 1.1rem;
		margin: 0;
	}
	.evidence-item {
		font-family: var(--font-body, "Source Sans 3", system-ui, sans-serif);
		font-size: 0.8rem;
		line-height: 1.5;
		color: var(--alt-slate, #666);
	}
	.evidence-id {
		font-family: var(--font-mono, "IBM Plex Mono", ui-monospace, monospace);
		font-size: 0.72rem;
		font-weight: 600;
		color: var(--alt-charcoal, #1a1a1a);
	}
	.evidence-sep {
		margin: 0 0.35rem;
		color: var(--alt-ash, #999);
	}
	.evidence-label {
		color: var(--alt-primary, #2f4f4f);
	}

	.expand {
		margin-top: 0.7rem;
		padding-top: 0.55rem;
		border-top: 1px solid var(--surface-border, #c8c8c8);
		display: grid;
		gap: 0.6rem;
	}
	.expand-block {
		display: grid;
		gap: 0.25rem;
	}
	.expand-heading {
		font-family: var(--font-body, "Source Sans 3", system-ui, sans-serif);
		font-size: 0.6rem;
		font-weight: 700;
		letter-spacing: 0.12em;
		text-transform: uppercase;
		color: var(--alt-ash, #999);
		margin: 0;
	}
	.expand-text {
		font-family: var(--font-body, "Source Sans 3", system-ui, sans-serif);
		font-size: 0.9rem;
		line-height: 1.6;
		color: var(--alt-charcoal, #1a1a1a);
		margin: 0;
		max-width: 65ch;
	}
	.cta-row {
		display: flex;
		flex-wrap: wrap;
		gap: 0.4rem;
		margin-top: 0.2rem;
	}
	.cta {
		appearance: none;
		font-family: var(--font-mono, "IBM Plex Mono", ui-monospace, monospace);
		font-size: 0.72rem;
		font-weight: 600;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		padding: 0.4rem 0.75rem;
		background: var(--surface-bg, #faf9f7);
		color: var(--alt-charcoal, #1a1a1a);
		border: 1px solid var(--alt-charcoal, #1a1a1a);
		cursor: pointer;
		transition: background 120ms ease, color 120ms ease;
	}
	.cta:hover:not([disabled]) {
		background: var(--alt-charcoal, #1a1a1a);
		color: var(--surface-bg, #faf9f7);
	}
	.cta:focus-visible {
		outline: 2px solid var(--alt-charcoal, #1a1a1a);
		outline-offset: 2px;
	}
	.cta[disabled] {
		color: var(--alt-ash, #999);
		border-color: var(--surface-border, #c8c8c8);
		cursor: not-allowed;
	}
	.cta--dismiss {
		margin-left: auto;
		border-color: var(--alt-terracotta, #b85450);
		color: var(--alt-terracotta, #b85450);
	}
	.cta--dismiss:hover:not([disabled]) {
		background: var(--alt-terracotta, #b85450);
		color: var(--surface-bg, #faf9f7);
	}

	/* ADR-000914 "I got this" CTA — Newspaper Style graduation button.
	   Sepia-3 border on rest, sepia-4 fill on hover; same dimensions as
	   the other CTAs so the row stays uniform. No motion beyond color
	   transitions; prefers-reduced-motion drops those too. */
	.cta--internalize {
		border-color: #8a6f47;
		color: #5d4a2c;
		transition: background 160ms linear, color 160ms linear,
			border-color 160ms linear;
	}
	.cta--internalize:hover:not([disabled]) {
		background: #5d4a2c;
		border-color: #5d4a2c;
		color: var(--surface-bg, #faf9f7);
	}
	@media (prefers-reduced-motion: reduce) {
		.cta--internalize {
			transition: none;
		}
	}

	.entry-foot {
		margin-top: 0.7rem;
		padding-top: 0.5rem;
		border-top: 1px solid var(--surface-border, #c8c8c8);
		display: flex;
		gap: 1.2rem;
		font-family: var(--font-mono, "IBM Plex Mono", ui-monospace, monospace);
		font-size: 0.65rem;
		color: var(--alt-ash, #999);
	}
	.foot-cell--freshness {
		margin-left: auto;
	}
	.foot-cell--super {
		text-transform: uppercase;
		letter-spacing: 0.12em;
		color: var(--alt-terracotta, #b85450);
	}

	/* Depth hints (ADR-000831 §12). Alt-Paper forbids drop shadows, so depth
	   is expressed via saturate/brightness + rule weight. Reduced motion keeps
	   only the rule weight so the visual hierarchy remains. */
	.entry.depth-1 {
		filter: saturate(0.88) brightness(0.995);
	}
	.entry.depth-2 {
		filter: none;
	}
	.entry.depth-3 {
		filter: saturate(1.04);
		border-color: color-mix(in oklab, var(--alt-charcoal, #1a1a1a) 35%, var(--surface-border, #c8c8c8));
	}
	.entry.depth-4 {
		filter: saturate(1.06);
		border: 1.5px solid var(--alt-charcoal, #1a1a1a);
	}

	/* OODA Z-axis (canonical contract §12 — "deeper focus: 奥へ入る /
	   return: 手前に戻る"). Each tile's `data-stage` maps to a translateZ
	   band inside the foreground plane's perspective container. The eye
	   reads `Observe` as up-front and `Act` as committed (deepest), with
	   the cycle closing back to Observe via `act → observe` returns.
	   Animated transitioning is handled by the global `.entry`'s
	   `transition: transform 240ms` so a `transitionTo()` smoothly
	   slides the tile to its new Z position. */
	.entry[data-stage="observe"] {
		transform: translateZ(0px);
	}
	.entry[data-stage="orient"] {
		transform: translateZ(-12px);
	}
	.entry[data-stage="decide"] {
		transform: translateZ(-24px);
	}
	.entry[data-stage="act"] {
		transform: translateZ(-36px);
	}
	/* When the user expands a tile it has the user's focus — bring it back
	 * to the front of the perspective stack so its CTAs sit visually (and
	 * for hit-testing) above sibling planes / wrapper rows that are at z=0.
	 * Without this, the auto-orient transition (e90ada874) pushes the tile
	 * to translateZ(-12px) the instant the user taps, and inner CTAs
	 * (Revisit/Ask/Snooze/Dismiss) become unreachable to real (coordinate-
	 * based) clicks: every intermediate ancestor at z=0 wins hit-testing
	 * over the receded article. The collapsed-stage Z bands stay intact for
	 * sibling tiles in the bucket planes, which is where the OODA depth UX
	 * actually reads. */
	.entry.expanded {
		transform: translateZ(0px);
	}

	@media (prefers-reduced-motion: reduce) {
		.entry {
			animation: none;
			opacity: 1;
			transition: opacity 160ms ease;
		}
		.entry.depth-1,
		.entry.depth-2,
		.entry.depth-3,
		.entry.depth-4 {
			filter: none;
		}
		/* Reduced motion replaces Z translation with a flat tile and a
		   saturate accent that still encodes hierarchy (contract §5/§12.5
		   dissolve + highlight fade + color shift). */
		.entry[data-stage="observe"],
		.entry[data-stage="orient"],
		.entry[data-stage="decide"],
		.entry[data-stage="act"] {
			transform: none;
		}
		.entry[data-stage="orient"] {
			filter: saturate(0.96);
		}
		.entry[data-stage="decide"] {
			filter: saturate(0.92);
		}
		.entry[data-stage="act"] {
			filter: saturate(0.88);
		}
	}
</style>
