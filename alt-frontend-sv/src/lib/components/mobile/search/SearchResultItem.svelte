<script lang="ts">
import type { SearchFeedItem } from "#lib/schema/search.js";
import { sanitizeHrefUrl } from "#lib/utils/urlSafety.js";

interface Props {
	result: SearchFeedItem;
	onSelect?: (result: SearchFeedItem) => void;
}

const { result, onSelect }: Props = $props();

let isDescriptionExpanded = $state(false);

const descriptionText = $derived((result.description || "").trim());
const hasDescription = $derived(descriptionText.length > 0);
const shouldTruncateDescription = $derived(descriptionText.length > 200);
const displayDescription = $derived(
	isDescriptionExpanded
		? descriptionText
		: shouldTruncateDescription
			? `${descriptionText.slice(0, 200)}...`
			: descriptionText,
);

const authorName = $derived(
	result.author?.name || result.authors?.[0]?.name || "Unknown",
);
const publishedDate = $derived(
	result.published ? new Date(result.published).toLocaleDateString() : null,
);
</script>

<article class="archive-result" data-role="archive-result-item" data-testid="search-result-item">
	<a
		href={sanitizeHrefUrl(result.link) ?? "#"}
		target="_blank"
		rel="noopener noreferrer"
		class="result-title"
	>
		{result.title}
	</a>

	{#if result.published}
		<span class="result-dateline">
			{authorName}{publishedDate ? ` \u00b7 ${publishedDate}` : ""}
		</span>
	{/if}

	{#if hasDescription}
		<p class="result-excerpt">{displayDescription}</p>
		{#if shouldTruncateDescription}
			<button
				type="button"
				onclick={() => { isDescriptionExpanded = !isDescriptionExpanded; }}
				class="result-toggle"
			>
				{isDescriptionExpanded ? "SHOW LESS" : "READ MORE"}
			</button>
		{/if}
	{/if}

	{#if result.link}
		<div class="result-actions">
			<button
				type="button"
				onclick={() => onSelect?.(result)}
				class="result-action-btn"
				data-role="toggle-details-btn"
			>
				DETAILS
			</button>
		</div>
	{/if}
</article>

<style>
	/* Feed descriptions arrive as raw `content:encoded` blobs, so a single
	   CDN image URL (query string included) is routinely one unbreakable
	   token far wider than a phone viewport. min-width: 0 lets the card
	   shrink below that intrinsic width and `overflow-wrap: anywhere` on the
	   text below splits the token at any glyph; without both, the URL runs
	   straight out of the card and gives the whole page a horizontal
	   scrollbar. */
	.archive-result {
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
		min-width: 0;
		padding: 0.75rem;
		border: 1px solid var(--surface-border);
		background: var(--surface-bg);
		transition: background 0.15s;
	}

	.result-title {
		font-family: var(--font-display);
		font-size: 0.95rem;
		font-weight: 600;
		color: var(--alt-charcoal);
		text-decoration: none;
		line-height: 1.3;
		overflow-wrap: anywhere;
		word-break: break-word;
	}

	.result-title:hover {
		text-decoration: underline;
		text-underline-offset: 2px;
	}

	.result-dateline {
		font-family: var(--font-mono);
		font-size: 0.65rem;
		color: var(--alt-ash);
		letter-spacing: 0.04em;
		overflow-wrap: anywhere;
		word-break: break-word;
	}

	.result-excerpt {
		font-family: var(--font-body);
		font-size: 0.85rem;
		color: var(--alt-slate);
		line-height: 1.5;
		margin: 0;
		overflow-wrap: anywhere;
		word-break: break-word;
	}

	.result-toggle {
		font-family: var(--font-mono);
		font-size: 0.65rem;
		color: var(--alt-primary);
		background: transparent;
		border: none;
		cursor: pointer;
		padding: 0;
		letter-spacing: 0.04em;
		text-transform: uppercase;
	}

	.result-actions {
		display: flex;
		gap: 0.5rem;
		margin-top: 0.5rem;
		padding-top: 0.5rem;
		border-top: 1px solid var(--surface-border);
	}

	.result-actions .result-action-btn {
		flex: 1 1 0;
		min-width: 0;
	}

	.result-action-btn {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		gap: 0.4rem;
		width: 100%;
		min-height: 44px;
		padding: 0.5rem 1rem;
		font-family: var(--font-body);
		font-size: 0.75rem;
		font-weight: 600;
		letter-spacing: 0.04em;
		text-transform: uppercase;
		color: var(--alt-charcoal);
		background: transparent;
		border: 1.5px solid var(--alt-charcoal);
		cursor: pointer;
		transition: background 0.15s, color 0.15s;
	}

	.result-action-btn:hover {
		background: var(--alt-charcoal);
		color: var(--surface-bg);
	}
</style>
