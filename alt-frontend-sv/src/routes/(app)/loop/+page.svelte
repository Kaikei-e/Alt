<script lang="ts">
import { onDestroy, onMount } from "svelte";
import { flip } from "svelte/animate";
import { cubicOut } from "svelte/easing";
import { goto, invalidate } from "$app/navigation";
import { observeTiles } from "$lib/actions/observe-tiles";
import ChangedDiffCard from "$lib/components/knowledge-loop/ChangedDiffCard.svelte";
import ContinueStream from "$lib/components/knowledge-loop/ContinueStream.svelte";
import EmptyNow from "$lib/components/knowledge-loop/EmptyNow.svelte";
import LoopEntryTile from "$lib/components/knowledge-loop/LoopEntryTile.svelte";
import LoopPlaneStack from "$lib/components/knowledge-loop/LoopPlaneStack.svelte";
import type { PlaneKey } from "$lib/components/knowledge-loop/loop-plane-keys";
import OodaPipeline from "$lib/components/knowledge-loop/OodaPipeline.svelte";
import ReviewDock from "$lib/components/knowledge-loop/ReviewDock.svelte";
import type {
	KnowledgeLoopEntryData,
	KnowledgeLoopResult,
	LoopStageName,
} from "$lib/connect/knowledge_loop";
import { makeCoalescedRefresh } from "$lib/hooks/loop-coalesce";
import { makeFirstFrameSkipper } from "$lib/hooks/loop-stream-skip-first";
import { startVisibilityRecovery } from "$lib/hooks/loop-visibility-recovery";
import { useKnowledgeLoop } from "$lib/hooks/useKnowledgeLoop.svelte";
import { useKnowledgeLoopStream } from "$lib/hooks/useKnowledgeLoopStream.svelte";
import { resolveLoopSourceUrl } from "$lib/utils/loop-source-url";
import { loopRecede } from "$lib/transitions/loop-recede";
import { uuidv7 } from "$lib/utils/uuidv7";
import "$lib/styles/loop-depth.css";
import type { PageData } from "./$types";

let { data }: { data: PageData } = $props();

/**
 * Knowledge Loop — the state-machine navigation of knowledge. Shared
 * Alt-Paper vocabulary (serif display, monospace metadata, thin rules,
 * sharp edges, no shadows) is recomposed around this page's own
 * responsibility: tracking a user through Observe → Orient → Decide → Act,
 * rather than publishing reports (Acolyte) or consulting (Ask Augur).
 * See ADR-000831 for the state-machine contract.
 */

let revealed = $state(false);
onMount(() => {
	requestAnimationFrame(() => {
		revealed = true;
	});
});

const EMPTY_LOOP: KnowledgeLoopResult = {
	foregroundEntries: [],
	bucketEntries: [],
	surfaces: [],
	sessionState: undefined,
	overallServiceQuality: "unspecified",
	generatedAt: "",
	projectionSeqHiwater: 0,
};

// The loop hook owns its own reactive state; seeding it once from the
// SSR-loaded data.loop snapshot is the intended lifecycle here.
// svelte-ignore state_referenced_locally
const loop = useKnowledgeLoop({
	initial: data.loop ?? EMPTY_LOOP,
	// svelte-ignore state_referenced_locally
	lensModeId: data.lensModeId ?? "default",
});

// When `invalidate("loop:data")` re-runs the load function, `data.loop`
// updates with a fresh snapshot. Push it into the hook so the foreground
// and sessionState track the projector without forcing a full SSR re-render.
// The hook's `replaceSnapshot` preserves any optimistic local dismissals
// until the projector catches up.
//
// Reference-equality against the constructor's seed, not a boolean gate, so
// we still re-seed correctly if SSR initially returned `null` (unauthenticated)
// and a subsequent invalidation supplies the first real snapshot.
// svelte-ignore state_referenced_locally
const initialDataLoop = data.loop;
$effect(() => {
	const next = data.loop;
	if (!next) return;
	if (next === initialDataLoop) return; // Hook constructor already seeded.
	loop.replaceSnapshot(next);
});

const foreground = $derived(loop.entries);
const sessionState = $derived(loop.sessionState);
const quality = $derived(data.loop?.overallServiceQuality ?? "unspecified");

// Partition non-NOW entries into Continue / Changed / Review planes. The
// projector scopes each entry to exactly one bucket, so these three arrays
// never overlap. The plane stack itself stays mounted even when every bucket
// is empty so users still see the Loop's four surfaces.
const bucketEntries = $derived(loop.bucketEntries);
const continueEntries = $derived(
	bucketEntries.filter((e) => e.surfaceBucket === "continue"),
);
const changedEntries = $derived(
	bucketEntries.filter((e) => e.surfaceBucket === "changed"),
);
const reviewEntries = $derived(
	bucketEntries.filter((e) => e.surfaceBucket === "review"),
);

// Server-sent Loop updates (ADR-000831 §9). Stream frames are hints: on
// non-silent frames we ask SvelteKit to refresh just the `loop:data` resource
// instead of the whole page tree. The refresh is *coalesced*: a 600 ms
// trailing debounce + single-flight guard ensures a burst of frames (or a
// JWT-expiry loop) maps to at most one `__data.json` refetch per window.
//
// The pre-fix code called `invalidateAll()` from both `onFrame` and
// `onExpired`. That replaced `data` on every call, which churned the stream
// hook's `data`-keyed `$effect`, which tore down + reopened the stream, which
// hit immediate JWT expiry on the still-old SSR token, which fired
// `onExpired` again. Live nginx + alt-backend logs (2026-04-26 04:30) caught
// this as ~50 simultaneous `GetKnowledgeLoop` calls and ~50 lockstep
// `stream_jwt_expired` log lines per cycle, eventually tripping
// `ERR_INSUFFICIENT_RESOURCES` in the browser. The hook's own
// `scheduleReconnect` already owns reconnect-with-backoff, so we no longer
// need the page to react to expiry at all beyond the same coalesced refresh.
let streamEnabled = $state(false);
onMount(() => {
	streamEnabled = true;
});

const coalescedRefresh = makeCoalescedRefresh(async () => {
	await invalidate("loop:data");
});
onDestroy(() => coalescedRefresh.dispose());

// Tab-return recovery: if the user has backgrounded the tab for >30s and
// returns, any in-flight `/loop/transition` request is functionally lost
// (server JWT may have expired, the connection may have been reset, the
// await frame may have been bfcache-frozen). Without this, `inFlight` keys
// would stay set forever and `LoopEntryTile`'s `disabled={inFlight}` gate
// would lock the buttons. We also coalesce-refresh so the foreground
// snapshot reflects whatever the server moved to during the idle window.
let visibilityRecovery: { dispose: () => void } | null = null;
onMount(() => {
	visibilityRecovery = startVisibilityRecovery({
		thresholdMs: 30_000,
		onRecover() {
			loop.resetInFlight("visibility");
			coalescedRefresh.trigger();
		},
	});
});
onDestroy(() => visibilityRecovery?.dispose());

// Skip the first non-silent frame the stream emits after `onMount`.
// The server inlines the current snapshot into `data.loop` during SSR,
// then replays the same state as the first frame on every fresh
// subscription so reconnecting clients catch up. Re-invalidating on
// that frame is pure churn: SvelteKit re-runs the load function,
// allocates a new `data.loop` object, and the keyed `{#each}` over
// `foreground` re-keys against the new reference. Any first-click on
// an article tile that was mid-flight during the re-render is
// dropped, which is the production "first click does nothing until
// reload" symptom we caught on /loop. The skipper rearms on stream
// reconnect (`onExpired`) because the server replays state on the
// new connection.
const skipFirstStreamRefresh = makeFirstFrameSkipper(() => {
	coalescedRefresh.trigger();
});

useKnowledgeLoopStream({
	get enabled() {
		return streamEnabled;
	},
	get lensModeId() {
		return data.lensModeId ?? "default";
	},
	onFrame(frame) {
		// Silent updates per contract §9: revised/heartbeat do not disturb
		// foreground. Appended/superseded/withdrawn/rebalanced warrant a refetch
		// — coalesced so a burst maps to one network call, not one per frame.
		if (frame.kind === "heartbeat") return;
		if (loop.applyStreamFrame(frame)) return;
		if (frame.kind === "revised") return;
		skipFirstStreamRefresh();
	},
	onExpired() {
		// Don't kick the SSR refresh off the JWT-expiry path. The stream hook
		// schedules its own reconnect and the next non-silent frame on the
		// fresh stream will trigger the coalesced refresh anyway.
		skipFirstStreamRefresh.reset();
	},
});

// resolveSourceUrl is a wrapper around the shared `resolveLoopSourceUrl`
// helper so existing callers (LoopEntryTile, Review-lane) keep their
// signature. The helper enforces:
//   - actTargets[].sourceUrl (article) → safeArticleHref → returned
//   - whyPrimary.evidenceRefs[0].refId → safeArticleHref → fallback
//   - actTargets[].route is *display-only* (internal SPA path) and is
//     never returned as a URL — that conflation was the ACT Open bug.
function resolveSourceUrl(entry: KnowledgeLoopEntryData): string | null {
	return resolveLoopSourceUrl(entry);
}

// isSafeInternalPath stays for defense-in-depth: future Open targets that
// resolve to an internal SPA route (e.g. `/augur/<id>`) need a same-origin
// goto without the SPA-reader query handoff. With resolveSourceUrl now
// returning HTTPS-only output, the article-open path no longer reaches
// this branch — but Review-lane's onReviewOpen and any external `href`
// passed to onEntryOpen() may.
function isSafeInternalPath(href: string): boolean {
	return (
		href.startsWith("/") &&
		!href.startsWith("//") &&
		!href.startsWith("/\\") &&
		!href.includes(":")
	);
}

function onEntryOpen(entry: KnowledgeLoopEntryData, href?: string) {
	href = href ?? resolveSourceUrl(entry) ?? "";
	if (!href) return;
	if (isSafeInternalPath(href)) {
		void goto(href);
		return;
	}
	// External article URL → SPA reader view, not a new tab.
	// Knowledge Loop's "Act" deserves to land inside the app: the reader
	// supports summarisation, reading-time, and citation rails, and avoids
	// the popup-blocker race (window.open after async work). Mirrors the
	// pattern already used by `home/+page.svelte`.
	const params = new URLSearchParams();
	params.set("url", href);
	if (entry.whyPrimary.text) {
		params.set("title", entry.whyPrimary.text);
	}
	void goto(
		`/articles/${encodeURIComponent(entry.entryKey)}?${params.toString()}`,
	);
}

function onObserve(entryKey: string) {
	void loop.observe(entryKey);
}

const askInFlight = new Set<string>();

async function onAsk(entry: KnowledgeLoopEntryData): Promise<void> {
	if (askInFlight.has(entry.entryKey)) return;
	askInFlight.add(entry.entryKey);
	try {
		const res = await fetch("/loop/ask", {
			method: "POST",
			headers: { "content-type": "application/json" },
			body: JSON.stringify({
				// svelte-ignore state_referenced_locally
				lensModeId: data.lensModeId ?? "default",
				clientHandshakeId: uuidv7(),
				entryKey: entry.entryKey,
			}),
		});
		if (!res.ok) return;
		const json = (await res.json()) as { conversationId?: string };
		if (json.conversationId) {
			await goto(`/augur/${encodeURIComponent(json.conversationId)}`);
		}
	} finally {
		askInFlight.delete(entry.entryKey);
	}
}

function effectiveEntryStage(entry: KnowledgeLoopEntryData): LoopStageName {
	return entry.currentEntryStage ?? entry.proposedStage;
}

function nextStage(entry: KnowledgeLoopEntryData): LoopStageName {
	switch (effectiveEntryStage(entry)) {
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

function stageLabel(stage: LoopStageName): string {
	return (
		{
			observe: "Observe",
			orient: "Orient",
			decide: "Decide",
			act: "Act",
		} as const
	)[stage];
}

function advanceEntry(entry: KnowledgeLoopEntryData) {
	const from = effectiveEntryStage(entry);
	const to = nextStage(entry);
	if (!loop.canTransition(from, to)) return;
	void loop.transitionTo(entry.entryKey, to, "user_tap");
}

function onPipelineStageSelect(to: LoopStageName) {
	const entry = activeEntry;
	if (!entry) return;
	const from = effectiveEntryStage(entry);
	if (from === to || !loop.canTransition(from, to)) return;
	void loop.transitionTo(entry.entryKey, to, "user_tap");
}

function onWorkspaceOpen(entry: KnowledgeLoopEntryData) {
	const href = resolveSourceUrl(entry);
	if (href) {
		void onEntryOpen(entry, href);
	}
}

// Monospace byline parts. Intentionally en-dash separated for editorial
// readability; never longer than one visual row on mobile.
const stageName = $derived(sessionState?.currentStage ?? "observe");
let activePlaneKey = $state<PlaneKey>("now");
const activePlaneEntries = $derived(
	activePlaneKey === "now"
		? foreground
		: activePlaneKey === "continue"
			? continueEntries
			: activePlaneKey === "changed"
				? changedEntries
				: reviewEntries,
);
const activeEntry = $derived(
	activePlaneEntries[0] ??
		foreground[0] ??
		continueEntries[0] ??
		changedEntries[0] ??
		reviewEntries[0],
);
const selectedStageName = $derived(
	activeEntry ? effectiveEntryStage(activeEntry) : stageName,
);
const stageDisplay = $derived(stageLabel(selectedStageName));

// Graceful fallback for legacy projection rows that pre-date `source_url`
// in act_targets[]: when the helper cannot derive a public HTTPS URL,
// the Open button must visibly disable rather than navigate into a
// broken reader state.
const activeEntrySourceUrl = $derived(
	activeEntry ? resolveSourceUrl(activeEntry) : null,
);

const seqHi = $derived(data.loop?.projectionSeqHiwater ?? 0);
const lensId = $derived(data.lensModeId ?? "default");

// LoopPlaneStack input: descriptors for all four planes regardless of which
// are populated. Empty planes still appear in the stack as receding "edge
// peeks" so the user can see what bucket is currently quiet.
const planeDescriptors = $derived([
	{
		key: "now" as const,
		label: "Now",
		caption:
			foreground.length === 1 ? "1 in focus" : `${foreground.length} in focus`,
		count: foreground.length,
	},
	{
		key: "continue" as const,
		label: "Continue",
		caption:
			continueEntries.length === 1
				? "1 in motion"
				: `${continueEntries.length} in motion`,
		count: continueEntries.length,
	},
	{
		key: "changed" as const,
		label: "Changed",
		caption:
			changedEntries.length === 1
				? "1 revised"
				: `${changedEntries.length} revised`,
		count: changedEntries.length,
	},
	{
		key: "review" as const,
		label: "Review",
		caption:
			reviewEntries.length === 1
				? "1 to revisit"
				: `${reviewEntries.length} to revisit`,
		count: reviewEntries.length,
	},
]);

// transitionTo derives `from` from the entry's currentEntryStage fallback, so
// callers only supply the target stage + trigger. Each plane maps to the
// canonical next step per contract §7 allowed transitions.
function onContinueResume(entry: KnowledgeLoopEntryData) {
	advanceEntry(entry);
}
function onChangedConfirm(entry: KnowledgeLoopEntryData) {
	advanceEntry(entry);
}
function onReviewOpen(entry: KnowledgeLoopEntryData) {
	onEntryOpen(entry);
}

// Review-lane re-evaluation (fb.md §F). Routes the user's choice through
// `loop.reviewAction` which posts a same-stage transition with the matching
// recheck / archive / mark_reviewed trigger. The projector then patches
// dismiss_state per action so the next snapshot reflects the decision.
function onReviewAction(
	entry: KnowledgeLoopEntryData,
	action: "recheck" | "archive" | "mark_reviewed",
) {
	void loop.reviewAction(entry.entryKey, action);
}
</script>

<svelte:head>
	<title>Knowledge Loop — Alt</title>
</svelte:head>

<main
	class="loop-root loop-plane-root"
	class:revealed
	data-testid="knowledge-loop-root"
	data-stage={selectedStageName}
>
	<header class="loop-masthead">
		<OodaPipeline
			currentStage={selectedStageName}
			onStageSelect={onPipelineStageSelect}
		/>
		<h1 class="masthead-title">Knowledge Loop</h1>
		<p class="byline" aria-live="polite">
			<span class="byline-cell">
				<span class="byline-key">Lens</span>
				<span class="byline-val">{lensId}</span>
			</span>
			<span class="byline-sep">—</span>
			<span class="byline-cell">
				<span class="byline-key">Stage</span>
				<span class="byline-val">{stageDisplay}</span>
			</span>
			<span class="byline-sep">—</span>
			<span class="byline-cell">
				<span class="byline-key">Seq</span>
				<span class="byline-val">{seqHi}</span>
			</span>
		</p>
		<div class="masthead-rule" aria-hidden="true"></div>
	</header>

	{#if data.error}
		<aside class="quality-banner quality-banner--error" role="status">
			<span class="banner-label">Loop unavailable</span>
			<span class="banner-msg">{data.error}</span>
		</aside>
	{:else if quality !== "full" && quality !== "unspecified"}
		<aside class="quality-banner" role="status">
			<span class="banner-label">Service quality</span>
			<span class="banner-msg">{quality}</span>
		</aside>
	{/if}

	{#if !data.error}
		{#if activeEntry}
			<section
				class="ooda-workspace"
				data-testid="loop-ooda-workspace"
				data-stage={selectedStageName}
			>
				<div class="workspace-head">
					<span class="workspace-kicker">{activePlaneKey} / {stageDisplay}</span>
					<h2>{activeEntry.whyPrimary.text || activeEntry.entryKey}</h2>
				</div>
				<div class="workspace-body">
					{#if selectedStageName === "observe"}
						<!-- Observe: scan metadata, signal count, freshness date -->
						<div class="workspace-context workspace-context--observe">
							<p class="workspace-meta workspace-meta--signals">
								{#if activeEntry.freshnessAt}
									{new Date(activeEntry.freshnessAt).toLocaleDateString("en", { month: "short", day: "numeric" })}
								{/if}
								{#if activeEntry.loopPriority === "critical" || activeEntry.loopPriority === "continuing"}
									<span class="meta-sep">·</span>{activeEntry.loopPriority}
								{/if}
								{#if activeEntry.whyPrimary.evidenceRefs.length > 0}
									<span class="meta-sep">·</span>{activeEntry.whyPrimary.evidenceRefs.length} signal{activeEntry.whyPrimary.evidenceRefs.length === 1 ? "" : "s"}
								{/if}
							</p>
						</div>
					{:else if selectedStageName === "orient"}
						<!-- Orient: context expands with labelled evidence section -->
						<div class="workspace-context workspace-context--orient">
							{#if activeEntry.continueContext}
								<p class="workspace-context-label">Continue context</p>
								<p>{activeEntry.continueContext.summary}</p>
								{#if activeEntry.continueContext.recentActionLabels.length > 0}
									<p class="workspace-meta">{activeEntry.continueContext.recentActionLabels.join(" / ")}</p>
								{/if}
							{:else if activeEntry.changeSummary}
								<p class="workspace-context-label">Changed</p>
								<p>{activeEntry.changeSummary.summary}</p>
								{#if activeEntry.changeSummary.changedFields.length > 0}
									<p class="workspace-meta">{activeEntry.changeSummary.changedFields.join(" / ")}</p>
								{/if}
							{:else if activeEntry.sourceObservedAt}
								<p class="workspace-context-label">Observed</p>
								<p class="workspace-meta">{activeEntry.sourceObservedAt}</p>
							{:else}
								<p class="workspace-meta">{activeEntry.entryKey}</p>
							{/if}
						</div>
					{:else if selectedStageName === "decide"}
						<!-- Decide: editorial choice list when options exist -->
						{#if activeEntry.decisionOptions.length > 0}
							<ol class="decision-options" aria-label="Available actions">
								{#each activeEntry.decisionOptions as opt}
									<li class="decision-option">
										<button
											type="button"
											class="decision-btn"
											data-intent={opt.intent}
											onclick={() => advanceEntry(activeEntry)}
										>
											{opt.label ?? opt.actionId}
										</button>
									</li>
								{/each}
							</ol>
						{:else}
							<div class="workspace-context">
								{#if activeEntry.continueContext}
									<p>{activeEntry.continueContext.summary}</p>
								{:else if activeEntry.changeSummary}
									<p>{activeEntry.changeSummary.summary}</p>
								{:else}
									<p class="workspace-meta">{activeEntry.whyPrimary.text || activeEntry.entryKey}</p>
								{/if}
							</div>
						{/if}
					{:else if selectedStageName === "act"}
						<!-- Act: target confirmation before Open command -->
						{#if activeEntry.actTargets.length > 0}
							<div class="act-targets">
								{#each activeEntry.actTargets.slice(0, 2) as target}
									<div class="act-target" data-type={target.targetType}>
										<span class="act-target-type">{target.targetType}</span>
										<span class="act-target-ref">{target.route ?? target.targetRef}</span>
									</div>
								{/each}
							</div>
						{:else}
							<div class="workspace-context">
								{#if activeEntry.continueContext}
									<p>{activeEntry.continueContext.summary}</p>
								{:else}
									<p class="workspace-meta">{activeEntry.whyPrimary.text || activeEntry.entryKey}</p>
								{/if}
							</div>
						{/if}
					{/if}

					<div class="workspace-actions" aria-label="OODA commands">
						{#if selectedStageName === "act"}
							<button
								type="button"
								class="workspace-command"
								disabled={!activeEntrySourceUrl}
								aria-label={activeEntrySourceUrl ? "Open" : "Source URL unavailable"}
								onclick={() => onWorkspaceOpen(activeEntry)}
							>
								Open
							</button>
							<button
								type="button"
								class="workspace-command workspace-command--secondary"
								onclick={() => advanceEntry(activeEntry)}
							>
								Return
							</button>
						{:else if selectedStageName === "decide" && activeEntry.decisionOptions.length > 0}
							<button
								type="button"
								class="workspace-command workspace-command--secondary"
								onclick={() => onAsk(activeEntry)}
							>
								Ask
							</button>
						{:else}
							<button
								type="button"
								class="workspace-command"
								onclick={() => advanceEntry(activeEntry)}
							>
								{stageLabel(nextStage(activeEntry))}
							</button>
							<button
								type="button"
								class="workspace-command workspace-command--secondary"
								onclick={() => onAsk(activeEntry)}
							>
								Ask
							</button>
						{/if}
					</div>
				</div>
			</section>
		{/if}

		<LoopPlaneStack planes={planeDescriptors} bind:activeKey={activePlaneKey}>
			{#snippet plane(key)}
				{#if key === "now"}
					{#if foreground.length === 0}
						<EmptyNow />
					{:else}
						<div class="foreground-tiles" use:observeTiles={{ onObserve }}>
							{#each foreground as entry, i (entry.entryKey)}
								<div
									class="foreground-row"
									animate:flip={{ duration: 240, easing: cubicOut }}
									out:loopRecede={{ duration: 280 }}
								>
									<LoopEntryTile
										{entry}
										stagger={i}
										onTransition={loop.transitionTo}
										onDismiss={loop.dismiss}
										{onAsk}
										onOpen={onEntryOpen}
										canTransition={loop.canTransition}
										isInFlight={loop.isInFlight}
										{resolveSourceUrl}
									/>
								</div>
							{/each}
						</div>
					{/if}
				{:else if key === "continue"}
					{#if continueEntries.length === 0}
						<p class="plane-empty">Nothing in motion right now.</p>
					{:else}
						<ContinueStream
							entries={continueEntries}
							onResume={onContinueResume}
						/>
					{/if}
				{:else if key === "changed"}
					{#if changedEntries.length === 0}
						<p class="plane-empty">No revisions to review.</p>
					{:else}
						<ChangedDiffCard
							entries={changedEntries}
							onConfirm={onChangedConfirm}
						/>
					{/if}
				{:else if key === "review"}
					{#if reviewEntries.length === 0}
						<p class="plane-empty">Nothing waiting for review.</p>
					{:else}
						<ReviewDock
							entries={reviewEntries}
							onOpen={onReviewOpen}
							{onReviewAction}
						/>
					{/if}
				{/if}
			{/snippet}
		</LoopPlaneStack>
	{/if}
</main>

<style>
	.loop-root {
		max-width: 72ch;
		margin: 0 auto;
		padding: 1.2rem 1.1rem 3rem;
		background: var(--surface-bg, #faf9f7);
		color: var(--alt-charcoal, #1a1a1a);
		min-height: 100%;
		opacity: 0;
		transform: translateY(6px);
		transition:
			opacity 0.35s ease,
			transform 0.35s ease;
	}
	.loop-root.revealed {
		opacity: 1;
		transform: translateY(0);
	}

	.loop-masthead {
		margin-bottom: 1.5rem;
	}

	.ooda-workspace {
		display: grid;
		gap: 0.75rem;
		margin: 0 0 1.25rem;
		padding: 0.85rem 0;
		border-top: 2px solid var(--alt-charcoal, #1a1a1a);
		border-bottom: 1px solid var(--surface-border, #c8c8c8);
	}
	.workspace-head {
		display: grid;
		gap: 0.25rem;
	}
	.workspace-kicker {
		font-family: var(--font-mono, "IBM Plex Mono", ui-monospace, monospace);
		font-size: 0.64rem;
		font-weight: 700;
		letter-spacing: 0.12em;
		text-transform: uppercase;
		color: var(--alt-ash, #999);
	}
	.workspace-head h2 {
		margin: 0;
		font-family: var(--font-display, "Playfair Display", Georgia, serif);
		font-size: 1.15rem;
		line-height: 1.25;
		color: var(--alt-charcoal, #1a1a1a);
	}
	.workspace-body {
		display: grid;
		gap: 0.8rem;
	}
	.workspace-context {
		display: grid;
		gap: 0.25rem;
		min-width: 0;
	}
	.workspace-context p {
		margin: 0;
		font-family: var(--font-body, "Source Sans 3", system-ui, sans-serif);
		font-size: 0.86rem;
		line-height: 1.45;
		color: var(--alt-charcoal, #1a1a1a);
	}
	.workspace-context .workspace-meta {
		font-family: var(--font-mono, "IBM Plex Mono", ui-monospace, monospace);
		font-size: 0.68rem;
		color: var(--alt-slate, #666);
		overflow-wrap: anywhere;
	}
	.workspace-actions {
		display: flex;
		flex-wrap: wrap;
		gap: 0.55rem;
		align-items: center;
	}
	.workspace-command {
		appearance: none;
		border: 1px solid var(--alt-charcoal, #1a1a1a);
		border-radius: 0;
		background: var(--alt-charcoal, #1a1a1a);
		color: var(--surface-bg, #faf9f7);
		padding: 0.38rem 0.72rem;
		font-family: var(--font-mono, "IBM Plex Mono", ui-monospace, monospace);
		font-size: 0.68rem;
		font-weight: 700;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		cursor: pointer;
	}
	.workspace-command--secondary {
		background: transparent;
		color: var(--alt-charcoal, #1a1a1a);
	}
	.workspace-command:hover {
		background: var(--alt-terracotta, #b85450);
		border-color: var(--alt-terracotta, #b85450);
		color: var(--surface-bg, #faf9f7);
	}
	.workspace-command:focus-visible {
		outline: 2px solid var(--alt-terracotta, #b85450);
		outline-offset: 2px;
	}

	.foreground-tiles {
		display: grid;
		gap: 0.8rem;
		/* Local 3D context so each foreground row receives Z transforms from
		 * `out:loopRecede` against a shared vanishing point (perspective on
		 * `.loop-plane-root` in loop-depth.css). Without preserve-3d here, each
		 * tile renders flat and the Z-recede flattens into a 2D fade. */
		transform-style: preserve-3d;
	}
	.foreground-row {
		/* Each row participates in the parent's perspective — keep flat at rest;
		 * `out:loopRecede` adds translateZ during exit only. */
		transform-style: preserve-3d;
	}

	.plane-empty {
		margin: 0;
		padding: 0.4rem 0;
		font-family: var(--font-mono, "IBM Plex Mono", ui-monospace, monospace);
		font-size: 0.72rem;
		color: var(--alt-ash, #999);
	}

	/* ── Stage-specific workspace panels ─────────────────────────────────── */

	/* Orient: heavier top rule signals entry into context mode */
	.ooda-workspace[data-stage="orient"] {
		border-top-width: 3px;
	}

	/* Act: tinted surface beneath the command area */
	.ooda-workspace[data-stage="act"] {
		background: rgba(26, 26, 26, 0.025);
		padding-inline: 0.6rem;
		margin-inline: -0.6rem;
	}

	.workspace-context-label {
		margin: 0 0 0.2rem;
		font-family: var(--font-mono, "IBM Plex Mono", ui-monospace, monospace);
		font-size: 0.58rem;
		font-weight: 700;
		letter-spacing: 0.14em;
		text-transform: uppercase;
		color: var(--alt-ash, #999);
	}

	.workspace-meta--signals {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.3rem;
	}
	.meta-sep {
		color: var(--alt-ash, #999);
	}

	/* ── Decision options (Decide stage) ─────────────────────────────────── */
	.decision-options {
		list-style: none;
		margin: 0;
		padding: 0;
		display: grid;
		gap: 0.3rem;
		counter-reset: decision;
	}
	.decision-option {
		counter-increment: decision;
		display: flex;
		align-items: stretch;
	}
	.decision-option::before {
		content: counter(decision, upper-roman) ".";
		font-family: var(--font-mono, "IBM Plex Mono", ui-monospace, monospace);
		font-size: 0.6rem;
		color: var(--alt-ash, #999);
		width: 1.6rem;
		flex-shrink: 0;
		display: flex;
		align-items: center;
		padding-top: 0.05rem;
	}
	.decision-btn {
		appearance: none;
		background: transparent;
		border: 1px solid var(--surface-border, #c8c8c8);
		border-radius: 0;
		padding: 0.35rem 0.65rem;
		text-align: left;
		font-family: var(--font-body, "Source Sans 3", system-ui, sans-serif);
		font-size: 0.82rem;
		line-height: 1.3;
		color: var(--alt-charcoal, #1a1a1a);
		cursor: pointer;
		flex: 1;
		transition:
			border-color 160ms ease,
			background-color 160ms ease;
	}
	.decision-btn:hover {
		border-color: var(--alt-charcoal, #1a1a1a);
		background: rgba(0, 0, 0, 0.03);
	}
	.decision-btn:focus-visible {
		outline: 2px solid var(--alt-terracotta, #b85450);
		outline-offset: 2px;
	}
	.decision-btn[data-intent="open"]:hover {
		border-color: var(--alt-terracotta, #b85450);
	}

	/* ── Act targets (Act stage) ─────────────────────────────────────────── */
	.act-targets {
		display: grid;
		gap: 0.3rem;
	}
	.act-target {
		display: grid;
		grid-template-columns: 4.5rem 1fr;
		gap: 0.5rem;
		align-items: baseline;
		padding: 0.35rem 0.55rem;
		border: 1px solid var(--surface-border, #c8c8c8);
		background: var(--surface-bg, #faf9f7);
	}
	.act-target-type {
		font-family: var(--font-mono, "IBM Plex Mono", ui-monospace, monospace);
		font-size: 0.58rem;
		font-weight: 700;
		letter-spacing: 0.1em;
		text-transform: uppercase;
		color: var(--alt-ash, #999);
	}
	.act-target-ref {
		font-family: var(--font-mono, "IBM Plex Mono", ui-monospace, monospace);
		font-size: 0.7rem;
		color: var(--alt-charcoal, #1a1a1a);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.masthead-title {
		font-family: var(--font-display, "Playfair Display", Georgia, serif);
		font-size: clamp(2rem, 5.5vw, 3.1rem);
		font-weight: 800;
		line-height: 1.05;
		letter-spacing: -0.01em;
		color: var(--alt-charcoal, #1a1a1a);
		margin: 0.35rem 0 0.55rem;
	}

	.byline {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: 0.5rem;
		font-family: var(--font-mono, "IBM Plex Mono", ui-monospace, monospace);
		font-size: 0.7rem;
		color: var(--alt-slate, #666);
		margin: 0 0 0.7rem;
	}
	.byline-cell {
		display: inline-flex;
		align-items: baseline;
		gap: 0.35rem;
	}
	.byline-key {
		font-size: 0.6rem;
		font-weight: 600;
		letter-spacing: 0.1em;
		text-transform: uppercase;
		color: var(--alt-ash, #999);
	}
	.byline-val {
		color: var(--alt-charcoal, #1a1a1a);
	}
	.byline-sep {
		color: var(--surface-border, #c8c8c8);
	}

	.masthead-rule {
		height: 1px;
		background: var(--alt-charcoal, #1a1a1a);
		margin-top: 0.3rem;
	}

	.quality-banner {
		display: flex;
		align-items: baseline;
		gap: 0.75rem;
		padding: 0.6rem 0.9rem;
		margin: 0 0 1.4rem;
		border-left: 3px solid var(--alt-sand, #d4a574);
		background: var(--surface-2, #f5f4f1);
	}
	.quality-banner--error {
		border-left-color: var(--alt-terracotta, #b85450);
	}
	.banner-label {
		font-family: var(--font-body, "Source Sans 3", system-ui, sans-serif);
		font-size: 0.6rem;
		font-weight: 700;
		letter-spacing: 0.12em;
		text-transform: uppercase;
		color: var(--alt-ash, #999);
	}
	.banner-msg {
		font-family: var(--font-mono, "IBM Plex Mono", ui-monospace, monospace);
		font-size: 0.75rem;
		color: var(--alt-charcoal, #1a1a1a);
	}

	@media (prefers-reduced-motion: reduce) {
		.loop-root {
			transition: opacity 160ms ease;
			transform: none;
		}
	}

	@media (min-width: 768px) {
		.loop-root {
			padding: 2rem 1.5rem 4rem;
		}
		.ooda-workspace {
			grid-template-columns: minmax(0, 1.15fr) minmax(18rem, 0.85fr);
			align-items: start;
		}
		.workspace-body {
			border-left: 1px solid var(--surface-border, #c8c8c8);
			padding-left: 1rem;
		}
	}
</style>
