<script lang="ts">
import { onMount } from "svelte";
import { goto, invalidateAll } from "$app/navigation";
import LoopSurfacePlane from "$lib/components/knowledge-loop/LoopSurfacePlane.svelte";
import LoopEntryTile from "$lib/components/knowledge-loop/LoopEntryTile.svelte";
import EmptyNow from "$lib/components/knowledge-loop/EmptyNow.svelte";
import ContinueStream from "$lib/components/knowledge-loop/ContinueStream.svelte";
import ChangedDiffCard from "$lib/components/knowledge-loop/ChangedDiffCard.svelte";
import ReviewDock from "$lib/components/knowledge-loop/ReviewDock.svelte";
import { useKnowledgeLoop } from "$lib/hooks/useKnowledgeLoop.svelte";
import { useKnowledgeLoopStream } from "$lib/hooks/useKnowledgeLoopStream.svelte";
import { observeTiles } from "$lib/actions/observe-tiles";
import { uuidv7 } from "$lib/utils/uuidv7";
import type {
	KnowledgeLoopEntryData,
	KnowledgeLoopResult,
} from "$lib/connect/knowledge_loop";
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

// Server-sent Loop updates (ADR-000831 §9). Stream frames are hints: on every
// non-silent frame we invalidate the SSR snapshot so the next load() refetches
// foreground + surfaces from GetKnowledgeLoop. The stream is read-only — it
// never mutates projection state, matching immutable-design-guard F3.
let streamEnabled = $state(false);
onMount(() => {
	streamEnabled = true;
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
		// foreground. Appended/superseded/withdrawn/rebalanced warrant a refetch.
		if (frame.kind === "revised" || frame.kind === "heartbeat") return;
		void invalidateAll();
	},
	onExpired() {
		void invalidateAll();
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
		<div class="kicker-row" aria-hidden="true">
			<span class="kicker" class:kicker--active={stageName === "observe"}
				>Observe</span
			>
			<span class="kicker-sep">·</span>
			<span class="kicker" class:kicker--active={stageName === "orient"}
				>Orient</span
			>
			<span class="kicker-sep">·</span>
			<span class="kicker" class:kicker--active={stageName === "decide"}
				>Decide</span
			>
			<span class="kicker-sep">·</span>
			<span class="kicker" class:kicker--active={stageName === "act"}>Act</span>
		</div>
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

	{#if !data.error && foreground.length === 0}
		<EmptyNow />
	{:else if !data.error}
		<LoopSurfacePlane
			plane="foreground"
			label="Foreground"
			caption="{foreground.length} in focus"
		>
			<div class="foreground-tiles" use:observeTiles={{ onObserve }}>
				{#each foreground as entry, i (entry.entryKey)}
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
				{/each}
			</div>
		</LoopSurfacePlane>
	{/if}

	{#if hasBucketPlanes}
		{#if continueEntries.length > 0}
			<LoopSurfacePlane
				plane="mid-context"
				label="Continue"
				caption="{continueEntries.length} in motion"
			>
				<ContinueStream
					entries={continueEntries}
					onResume={onContinueResume}
				/>
			</LoopSurfacePlane>
		{/if}
		{#if changedEntries.length > 0}
			<LoopSurfacePlane
				plane="mid-context"
				label="Changed"
				caption="{changedEntries.length} revised"
			>
				<ChangedDiffCard
					entries={changedEntries}
					onConfirm={onChangedConfirm}
				/>
			</LoopSurfacePlane>
		{/if}
		{#if reviewEntries.length > 0}
			<LoopSurfacePlane
				plane="deep-focus"
				label="Review"
				caption="{reviewEntries.length} to revisit"
			>
				<ReviewDock entries={reviewEntries} onOpen={onReviewOpen} />
			</LoopSurfacePlane>
		{/if}
	{:else if bucketIndex.length > 0}
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
	}

	.kicker-row {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: 0.35rem;
		font-family: var(--font-body, "Source Sans 3", system-ui, sans-serif);
		font-size: 0.6rem;
		font-weight: 700;
		letter-spacing: 0.16em;
		text-transform: uppercase;
		color: var(--alt-ash, #999);
	}
	.kicker--active {
		color: var(--alt-charcoal, #1a1a1a);
	}
	.kicker-sep {
		color: var(--surface-border, #c8c8c8);
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
