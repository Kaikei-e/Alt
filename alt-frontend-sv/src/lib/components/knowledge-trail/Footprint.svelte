<script lang="ts">
import type { FootprintData } from "$lib/connect/knowledge_trail";

interface Props {
	footprint: FootprintData;
}

const { footprint }: Props = $props();

const VERB_LABEL: Record<string, string> = {
	read: "Read",
	asked: "Asked",
	returned: "Returned",
	listened: "Listened",
	dismissed: "Dismissed",
};

const verbLabel = $derived(VERB_LABEL[footprint.verb] ?? footprint.verb);
// Link to the in-app article reader by id only. The reader resolves the source
// URL from the id server-side (GetArticleSourceURL) — no URL in the link, no
// external href, no client-supplied URL.
const articleId = $derived(
	footprint.itemKey.startsWith("article:") ? footprint.itemKey.slice(8) : null,
);
const wear = $derived(
	footprint.wear === "deep" || footprint.wear === "worn"
		? footprint.wear
		: "thin",
);
const timeLabel = $derived(formatTime(footprint.occurredAt));
const displayTags = $derived(
	footprint.tags.filter((t) => t.trim() !== "").slice(0, 3),
);

function formatTime(iso: string): string {
	const d = new Date(iso);
	if (Number.isNaN(d.getTime())) return "";
	return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}
</script>

<div class="step" data-testid="trail-footprint" data-wear={wear}>
	<div class="spine-cell">
		<div class="spine-node {wear}"></div>
		<div class="spine-seg {wear}"></div>
	</div>
	<div class="footprint">
		<div class="fp-meta">
			<span class="fp-time">{timeLabel}</span>
			<span class="fp-verb">{verbLabel}</span>
			{#if footprint.contactCount > 1}
				<!-- D24: repeated contacts collapse into one row with a count. -->
				<span class="fp-count" data-testid="footprint-count"
					>× {footprint.contactCount} visits</span
				>
			{/if}
			{#if footprint.note}
				<span class="fp-note">{footprint.note}</span>
			{/if}
		</div>
		{#if articleId}
			<a class="fp-title fp-title-link" href="/articles/{articleId}" data-testid="footprint-link">{footprint.title || footprint.itemKey}</a>
		{:else}
			<div class="fp-title">{footprint.title || footprint.itemKey}</div>
		{/if}
		{#if footprint.excerpt}
			<p class="fp-excerpt">{footprint.excerpt}</p>
		{/if}
		{#if displayTags.length > 0}
			<div class="fp-tags">
				{#each displayTags as tag (tag)}
					<span class="fp-tag">{tag}</span>
				{/each}
			</div>
		{/if}
	</div>
</div>

<style>
	.step {
		display: flex;
		gap: 1.1rem;
	}
	.spine-cell {
		width: 2.4rem;
		flex: none;
		display: flex;
		flex-direction: column;
		align-items: center;
	}
	/* Path wear: the trail to an item is dotted/thin on first pass, solid when
	   worn (revisited), and bold when deep (revisited + asked). No numbers. */
	.spine-node {
		flex: none;
		margin: 0.2rem 0;
	}
	.spine-node.thin {
		width: 0.7rem;
		height: 0.7rem;
		border-radius: 50%;
		background: var(--surface-bg, #faf9f7);
		border: 2px solid var(--wear-worn, #8a8273);
	}
	.spine-node.worn {
		width: 0.8rem;
		height: 0.8rem;
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
	.footprint {
		flex: 1;
		min-width: 0;
		padding: 0.1rem 0 1.1rem;
	}
	.fp-meta {
		display: flex;
		align-items: center;
		gap: 0.6rem;
	}
	.fp-time {
		font-family: var(--font-mono);
		font-size: 0.7rem;
		color: var(--alt-ash, #999);
		letter-spacing: 0.04em;
	}
	.fp-verb {
		font-family: var(--font-mono);
		font-size: 0.66rem;
		font-weight: 600;
		letter-spacing: 0.12em;
		text-transform: uppercase;
		color: var(--alt-slate, #666);
	}
	.fp-count {
		font-family: var(--font-mono);
		font-size: 0.66rem;
		font-weight: 600;
		letter-spacing: 0.06em;
		color: var(--wear-deep, #3e3a32);
	}
	.fp-note {
		font-family: var(--font-mono);
		font-size: 0.68rem;
		color: var(--alt-ash, #999);
		font-style: italic;
	}
	.fp-title {
		font-family: var(--font-display);
		font-size: 1.05rem;
		font-weight: 600;
		line-height: 1.3;
		margin-top: 0.2rem;
		color: var(--alt-charcoal, #1a1a1a);
	}
	.fp-title-link {
		display: block;
		text-decoration: none;
		cursor: pointer;
	}
	.fp-title-link:hover {
		text-decoration: underline;
		color: var(--interactive-text, #2f4f4f);
	}
	.fp-excerpt {
		font-size: 0.84rem;
		color: var(--text-secondary, #333);
		line-height: 1.5;
		margin-top: 0.25rem;
		max-width: 60ch;
	}
	.fp-tags {
		display: flex;
		gap: 0.35rem;
		margin-top: 0.4rem;
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
</style>
