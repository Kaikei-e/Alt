<script lang="ts">
import type {
	BranchData,
	EpisodeData,
	FootprintData,
	ResolveBranchHandler,
} from "$lib/connect/knowledge_trail";
import BranchCard from "./BranchCard.svelte";
import Footprint from "./Footprint.svelte";

interface Props {
	episode: EpisodeData;
	/** Item keys matched by an active trail search (D25); drives auto-expand + highlight. */
	matchedItemKeys?: string[];
	/**
	 * The single branch (D26/D28) anchored on a member of this episode, if
	 * any — matched by the caller (first match only, one per episode). Absent
	 * means this episode carries no open branch; the branch inbox that used
	 * to sit under the spine is gone.
	 */
	branch?: BranchData;
	onResolveBranch?: ResolveBranchHandler;
}

const {
	episode,
	matchedItemKeys = [],
	branch,
	onResolveBranch,
}: Props = $props();

// Auto-expand while a search hit lives in this episode, without permanently
// overriding a manual toggle: `manualExpanded` is null until the user clicks,
// after which it wins over the search-driven default.
let manualExpanded = $state<boolean | null>(null);
const isSearchHit = $derived(
	matchedItemKeys.length > 0 &&
		episode.footprints.some((fp) => matchedItemKeys.includes(fp.itemKey)),
);
const expanded = $derived(manualExpanded ?? isSearchHit);

const VERB_LABEL: Record<string, string> = {
	read: "Read",
	asked: "Asked",
	returned: "Returned",
	listened: "Listened",
	dismissed: "Dismissed",
};
// "asked" counts in questions, everything else counts in visits ("times").
const VERB_NOUN: Record<string, [string, string]> = {
	asked: ["question", "questions"],
};
// Fixed reading order for the summary, independent of which member is newest —
// matches the field-notes mock's "Read N times · asked M questions" phrasing.
const VERB_PRIORITY = ["read", "returned", "asked", "listened", "dismissed"];

const wear = $derived(
	episode.wear === "deep" || episode.wear === "worn" ? episode.wear : "thin",
);

// The representative is the newest footprint (episode.footprints is newest
// first, per the wire contract) — episodes always carry at least one member.
const representative = $derived.by(() => {
	const first = episode.footprints[0];
	if (!first) {
		throw new Error("EpisodeCard requires at least one member footprint");
	}
	return first;
});

const articleId = $derived(
	representative.itemKey.startsWith("article:")
		? representative.itemKey.slice(8)
		: null,
);

const displayTags = $derived(
	representative.tags.filter((t) => t.trim() !== "").slice(0, 3),
);

function formatDate(iso: string): string {
	const d = new Date(iso);
	if (Number.isNaN(d.getTime())) return "";
	return d.toLocaleDateString([], { month: "short", day: "numeric" });
}

const newestLabel = $derived(
	formatDate(
		episode.footprints.reduce(
			(latest, fp) => (fp.occurredAt > latest ? fp.occurredAt : latest),
			representative.occurredAt,
		),
	),
);
const oldestLabel = $derived(
	formatDate(
		episode.footprints.reduce((earliest, fp) => {
			const first = fp.firstOccurredAt || fp.occurredAt;
			return first < earliest ? first : earliest;
		}, representative.firstOccurredAt || representative.occurredAt),
	),
);
// Oldest → newest: the range reads chronologically, left to right.
const dateRange = $derived(
	oldestLabel && oldestLabel !== newestLabel
		? `${oldestLabel} – ${newestLabel}`
		: newestLabel,
);

function summarizeContacts(footprints: FootprintData[]): string {
	const totals = new Map<string, number>();
	for (const fp of footprints) {
		totals.set(
			fp.verb,
			(totals.get(fp.verb) ?? 0) + Math.max(1, fp.contactCount),
		);
	}
	const order = [
		...VERB_PRIORITY.filter((v) => totals.has(v)),
		...[...totals.keys()].filter((v) => !VERB_PRIORITY.includes(v)),
	];
	return order
		.map((verb, i) => {
			const count = totals.get(verb) ?? 0;
			const label = VERB_LABEL[verb] ?? verb;
			const [singular, plural] = VERB_NOUN[verb] ?? ["time", "times"];
			const noun = count === 1 ? singular : plural;
			return `${i === 0 ? label : label.toLowerCase()} ${count} ${noun}`;
		})
		.join(" · ");
}

const contactSummary = $derived(summarizeContacts(episode.footprints));

function toggle() {
	manualExpanded = !expanded;
}
</script>

<div class="episode-step" data-testid="trail-episode" data-wear={wear}>
	<div class="spine-cell">
		<span class="spine-landmark">{newestLabel}</span>
		<div class="spine-node {wear}"></div>
		<div class="spine-seg {wear}"></div>
	</div>
	<article class="episode">
		{#if episode.thumbnailUrl}
			<img
				class="episode-thumb"
				data-testid="episode-thumbnail"
				src={episode.thumbnailUrl}
				alt=""
			/>
		{/if}
		<div class="episode-body">
			<div class="episode-head">
				{#if articleId}
					<a
						class="episode-title"
						data-testid="episode-link"
						href="/articles/{articleId}"
					>
						{representative.title || representative.itemKey}
					</a>
				{:else}
					<h3 class="episode-title">
						{representative.title || representative.itemKey}
					</h3>
				{/if}
				<button
					class="episode-expand"
					data-testid="episode-toggle"
					aria-expanded={expanded}
					onclick={toggle}
				>
					{expanded ? "▾" : "▸"}
					{episode.footprints.length} footprint{episode.footprints.length === 1
						? ""
						: "s"}
				</button>
			</div>
			<div class="episode-meta-row">
				<span class="episode-dates" data-testid="episode-dates">{dateRange}</span>
				<span class="episode-contact" data-testid="episode-contact"
					>{contactSummary}</span
				>
			</div>
			{#if displayTags.length > 0}
				<div class="episode-tags">
					{#each displayTags as tag (tag)}
						<span class="fp-tag" data-testid="episode-tag">{tag}</span>
					{/each}
				</div>
			{/if}
			{#if expanded}
				<div class="episode-footprints">
					{#each episode.footprints as fp (fp.footprintKey)}
						<Footprint
							footprint={fp}
							isHit={matchedItemKeys.includes(fp.itemKey)}
						/>
					{/each}
				</div>
			{/if}
			{#if branch && onResolveBranch}
				<div class="episode-branch" data-testid="episode-branch">
					<span class="episode-branch-kicker">Next step on this trail</span>
					<BranchCard
						branch={branch}
						testId="episode-branch-card"
						onResolve={onResolveBranch}
					/>
				</div>
			{/if}
		</div>
	</article>
</div>

<style>
	.episode-step {
		display: flex;
		gap: 1.1rem;
	}
	.spine-cell {
		width: 3.4rem;
		flex: none;
		display: flex;
		flex-direction: column;
		align-items: center;
	}
	.spine-landmark {
		font-family: var(--font-mono);
		font-size: 0.6rem;
		letter-spacing: 0.05em;
		color: var(--alt-ash, #999);
		margin-bottom: 0.3rem;
		white-space: nowrap;
	}
	.spine-node {
		flex: none;
		margin: 0.15rem 0;
	}
	.spine-node.thin {
		width: 0.85rem;
		height: 0.85rem;
		border-radius: 50%;
		background: var(--surface-bg, #faf9f7);
		border: 2px solid var(--wear-worn, #8a8273);
	}
	.spine-node.worn {
		width: 0.85rem;
		height: 0.85rem;
		border-radius: 50%;
		background: var(--wear-worn, #8a8273);
		border: 2px solid var(--wear-worn, #8a8273);
	}
	.spine-node.deep {
		width: 0.78rem;
		height: 0.78rem;
		border-radius: 1px;
		transform: rotate(45deg);
		background: var(--wear-deep, #3e3a32);
		border: 2px solid var(--wear-deep, #3e3a32);
	}
	.spine-seg {
		flex: 1;
		width: 0;
		min-height: 0.7rem;
	}
	.spine-seg.thin {
		border-left: 2px dotted var(--wear-thin, #c4beb2);
	}
	.spine-seg.worn {
		border-left: 3px solid var(--wear-worn, #8a8273);
	}
	.spine-seg.deep {
		border-left: 5px solid var(--wear-deep, #3e3a32);
	}
	.episode {
		flex: 1;
		min-width: 0;
		display: flex;
		gap: 1rem;
		padding: 0.85rem 0 1.35rem;
	}
	.episode-thumb {
		width: 92px;
		height: 66px;
		flex: none;
		object-fit: cover;
		background: var(--surface-2, #f5f4f1);
		border: 1px solid var(--surface-border, #c8c8c8);
	}
	.episode-body {
		flex: 1;
		min-width: 0;
	}
	.episode-head {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 0.8rem;
	}
	.episode-title {
		font-family: var(--font-display);
		font-size: 1.15rem;
		font-weight: 700;
		line-height: 1.3;
		margin: 0;
		color: var(--alt-charcoal, #1a1a1a);
		cursor: pointer;
		text-decoration: none;
	}
	a.episode-title:hover {
		color: var(--interactive-text-hover, #223b3b);
		text-decoration: underline;
		text-decoration-thickness: 1px;
		text-underline-offset: 3px;
	}
	.episode-expand {
		flex: none;
		font-family: var(--font-mono);
		font-size: 0.72rem;
		color: var(--interactive-text, #2f4f4f);
		background: none;
		border: none;
		cursor: pointer;
		padding: 0.1rem 0;
		white-space: nowrap;
	}
	.episode-expand:hover {
		color: var(--interactive-text-hover, #223b3b);
		text-decoration: underline;
	}
	.episode-meta-row {
		display: flex;
		align-items: center;
		gap: 0.7rem;
		margin-top: 0.35rem;
		flex-wrap: wrap;
	}
	.episode-dates {
		font-family: var(--font-mono);
		font-size: 0.72rem;
		color: var(--alt-ash, #999);
		letter-spacing: 0.03em;
	}
	.episode-contact {
		font-family: var(--font-body);
		font-size: 0.82rem;
		color: var(--text-secondary, #333);
	}
	.episode-tags {
		display: flex;
		gap: 0.35rem;
		margin-top: 0.55rem;
		flex-wrap: wrap;
	}
	.fp-tag {
		font-family: var(--font-mono);
		font-size: 0.68rem;
		color: var(--chip-text, #49443d);
		border: 1px solid var(--chip-border, #d0c8bb);
		background: var(--chip-bg, #ebe7df);
		padding: 0.08rem 0.5rem;
	}
	.episode-footprints {
		margin-top: 0.75rem;
		padding-top: 0.6rem;
		border-top: 1px dashed var(--surface-border, #c8c8c8);
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}
	.episode-branch {
		margin-top: 0.85rem;
	}
	.episode-branch-kicker {
		display: block;
		font-family: var(--font-mono);
		font-size: 0.66rem;
		font-weight: 600;
		letter-spacing: 0.1em;
		text-transform: uppercase;
		color: var(--alt-ash, #999);
		margin-bottom: 0.4rem;
	}

	@media (max-width: 620px) {
		.episode {
			flex-direction: column;
		}
		.episode-thumb {
			width: 100%;
			height: 84px;
		}
	}
</style>
