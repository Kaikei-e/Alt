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
import LoopSurfacePlane from "$lib/components/knowledge-loop/LoopSurfacePlane.svelte";
import OodaPipeline from "$lib/components/knowledge-loop/OodaPipeline.svelte";
import ReviewDock from "$lib/components/knowledge-loop/ReviewDock.svelte";
import type {
	KnowledgeLoopEntryData,
	KnowledgeLoopResult,
} from "$lib/connect/knowledge_loop";
import { makeCoalescedRefresh } from "$lib/hooks/loop-coalesce";
import { useKnowledgeLoop } from "$lib/hooks/useKnowledgeLoop.svelte";
import { useKnowledgeLoopStream } from "$lib/hooks/useKnowledgeLoopStream.svelte";
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
const surfaces = $derived(data.loop?.surfaces ?? []);
const quality = $derived(data.loop?.overallServiceQuality ?? "unspecified");

// Partition non-NOW entries into Continue / Changed / Review planes. The
// projector scopes each entry to exactly one bucket, so these three arrays
// never overlap. Empty arrays collapse their plane — the page stays quiet
// when the user has nothing queued in that surface (contract §14 empty-state).
const bucketEntries = $derived(data.loop?.bucketEntries ?? []);
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
		if (frame.kind === "revised" || frame.kind === "heartbeat") return;
		coalescedRefresh.trigger();
	},
	onExpired() {
		// Don't kick the SSR refresh off the JWT-expiry path. The stream hook
		// schedules its own reconnect and the next non-silent frame on the
		// fresh stream will trigger the coalesced refresh anyway.
	},
});

function resolveSourceUrl(entry: KnowledgeLoopEntryData): string | null {
	const article = entry.actTargets.find((t) => t.targetType === "article");
	if (article?.route) return article.route;
	const ref = entry.whyPrimary.evidenceRefs[0];
	if (ref?.refId && /^https?:\/\//i.test(ref.refId)) return ref.refId;
	return null;
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

// Monospace byline parts. Intentionally en-dash separated for editorial
// readability; never longer than one visual row on mobile.
const stageName = $derived(sessionState?.currentStage ?? "observe");
const stageDisplay = $derived(
	(
		{
			observe: "Observe",
			orient: "Orient",
			decide: "Decide",
			act: "Act",
		} as const
	)[stageName] ?? "Observe",
);

const seqHi = $derived(data.loop?.projectionSeqHiwater ?? 0);
const lensId = $derived(data.lensModeId ?? "default");

// Legacy bucket index — retained as a fallback when the server returns no
// bucket_entries (older build or degraded fetch path). The dedicated Surface
// plane components (ContinueStream / ChangedDiffCard / ReviewDock) supersede
// this view whenever bucket_entries is populated.
const bucketIndex = $derived(
	[
		{ bucket: "continue" as const, label: "Continue" },
		{ bucket: "changed" as const, label: "Changed" },
		{ bucket: "review" as const, label: "Review" },
	]
		.map(({ bucket, label }) => {
			const s = surfaces.find((s) => s.surfaceBucket === bucket);
			const count =
				(s?.primaryEntryKey ? 1 : 0) + (s?.secondaryEntryKeys?.length ?? 0);
			return { bucket, label, count };
		})
		.filter((x) => x.count > 0),
);
const hasBucketPlanes = $derived(bucketEntries.length > 0);

// LoopPlaneStack input: descriptors for all four planes regardless of which
// are populated. Empty planes still appear in the stack as receding "edge
// peeks" so the user can see what bucket is currently quiet.
const planeDescriptors = $derived([
	{
		key: "now" as const,
		label: "Now",
		caption:
			foreground.length === 1
				? "1 in focus"
				: `${foreground.length} in focus`,
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

let activePlaneKey = $state<PlaneKey>("now");

// transitionTo derives `from` from the entry's proposedStage, so callers only
// supply the target stage + trigger. Each plane maps to the canonical next
// step per contract §7 allowed transitions.
function onContinueResume(entry: KnowledgeLoopEntryData) {
	void loop.transitionTo(entry.entryKey, "decide", "user_tap");
}
function onChangedConfirm(entry: KnowledgeLoopEntryData) {
	void loop.transitionTo(entry.entryKey, "act", "user_tap");
}
function onReviewOpen(entry: KnowledgeLoopEntryData) {
	const href = resolveSourceUrl(entry);
	if (href) {
		void goto(href);
	}
}
</script>

<svelte:head>
	<title>Knowledge Loop — Alt</title>
</svelte:head>

<main
	class="loop-root loop-plane-root"
	class:revealed
	data-testid="knowledge-loop-root"
	data-stage={stageName}
>
	<header class="loop-masthead">
		<OodaPipeline currentStage={stageName} />
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

	{#if !data.error && hasBucketPlanes}
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
						<ReviewDock entries={reviewEntries} onOpen={onReviewOpen} />
					{/if}
				{/if}
			{/snippet}
		</LoopPlaneStack>
	{:else if !data.error && foreground.length === 0}
		<EmptyNow />
	{:else if !data.error}
		<LoopSurfacePlane
			plane="foreground"
			label="Foreground"
			caption="{foreground.length} in focus"
		>
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
							canTransition={loop.canTransition}
							isInFlight={loop.isInFlight}
							{resolveSourceUrl}
						/>
					</div>
				{/each}
			</div>
		</LoopSurfacePlane>

		{#if bucketIndex.length > 0}
			<nav class="bucket-index" aria-label="Other surface buckets">
				<span class="bucket-kicker">Also waiting</span>
				<ul class="bucket-list">
					{#each bucketIndex as b (b.bucket)}
						<li class="bucket-item">
							<span class="bucket-label">{b.label}</span>
							<span class="bucket-rule" aria-hidden="true"></span>
							<span class="bucket-count">{b.count}</span>
						</li>
					{/each}
				</ul>
			</nav>
		{/if}
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

	.bucket-index {
		margin-top: 2rem;
		padding-top: 0.9rem;
		border-top: 1px solid var(--surface-border, #c8c8c8);
	}
	.bucket-kicker {
		display: block;
		font-family: var(--font-body, "Source Sans 3", system-ui, sans-serif);
		font-size: 0.6rem;
		font-weight: 700;
		letter-spacing: 0.16em;
		text-transform: uppercase;
		color: var(--alt-ash, #999);
		margin-bottom: 0.55rem;
	}
	.bucket-list {
		list-style: none;
		padding: 0;
		margin: 0;
		display: grid;
		gap: 0.35rem;
	}
	.bucket-item {
		display: grid;
		grid-template-columns: auto 1fr auto;
		align-items: baseline;
		gap: 0.6rem;
	}
	.bucket-label {
		font-family: var(--font-display, "Playfair Display", Georgia, serif);
		font-size: 0.95rem;
		font-weight: 600;
		color: var(--alt-charcoal, #1a1a1a);
	}
	.bucket-rule {
		height: 1px;
		background: var(--surface-border, #c8c8c8);
		align-self: center;
		transform: translateY(1px);
	}
	.bucket-count {
		font-family: var(--font-mono, "IBM Plex Mono", ui-monospace, monospace);
		font-size: 0.72rem;
		color: var(--alt-slate, #666);
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
	}
</style>
