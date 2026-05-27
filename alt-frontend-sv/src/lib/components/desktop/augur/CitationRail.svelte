<script lang="ts">
import { ArrowUpRight } from "@lucide/svelte";
import { citationHref, type CitationKindName } from "./citation-href";

export type Citation = {
	URL: string;
	Title: string;
	PublishedAt?: string;
	Score?: number;
	Kind?: CitationKindName;
	RefID?: string;
};

type Props = {
	citations: Citation[];
	activeIndex?: number;
	onSelect?: (index: number) => void;
	loading?: boolean;
};

let {
	citations,
	activeIndex = -1,
	onSelect,
	loading = false,
}: Props = $props();

const months = [
	"JAN",
	"FEB",
	"MAR",
	"APR",
	"MAY",
	"JUN",
	"JUL",
	"AUG",
	"SEP",
	"OCT",
	"NOV",
	"DEC",
];

function formatDateline(iso?: string): string {
	if (!iso) return "";
	const d = new Date(iso);
	if (Number.isNaN(d.getTime())) return "";
	const m = months[d.getMonth()];
	const day = String(d.getDate()).padStart(2, "0");
	const hh = String(d.getHours()).padStart(2, "0");
	const mm = String(d.getMinutes()).padStart(2, "0");
	return `${m} ${day} · ${hh}:${mm}`;
}

function formatDomain(url: string): string {
	try {
		const u = new URL(url);
		return u.hostname.replace(/^www\./, "");
	} catch {
		return url;
	}
}

function pad2(n: number): string {
	return String(n).padStart(2, "0");
}

function handleSelect(i: number) {
	onSelect?.(i);
}

function handleKey(event: KeyboardEvent, i: number) {
	if (event.key === "Enter" || event.key === " ") {
		event.preventDefault();
		handleSelect(i);
	}
}

// hrefFor returns the click target for a citation, branching on Kind so a
// bare UUID never lands in `href` (where the browser would resolve it
// relative to /augur/<conversation_id>). Legacy / unknown kinds render
// without a link.
function hrefFor(c: Citation): string | undefined {
	return citationHref({
		kind: c.Kind ?? "UNSPECIFIED",
		url: c.URL ?? "",
		refId: c.RefID ?? "",
	});
}

// isExternal marks WEB citations that should open in a new tab. ARTICLE /
// SUMMARY links target same-origin /articles/<id> routes and must stay in the
// current tab so the SvelteKit client router can take over.
function isExternal(c: Citation): boolean {
	return c.Kind === "WEB";
}
</script>

<aside class="citation-rail" aria-label="Augur citations">
	<header class="rail-head">
		<h2 class="rail-title">Citations</h2>
		<div class="rail-rule" aria-hidden="true"></div>
	</header>

	{#if loading && citations.length === 0}
		<ol class="rail-list rail-skeleton" aria-hidden="true">
			{#each [0, 1, 2] as i (i)}
				<li class="rail-item is-skeleton">
					<span class="item-num">{pad2(i + 1)}</span>
					<div class="item-body">
						<div class="skeleton-line skeleton-line-title"></div>
						<div class="skeleton-line skeleton-line-meta"></div>
					</div>
				</li>
			{/each}
		</ol>
	{:else if citations.length === 0}
		<p class="rail-empty">No citations yet</p>
	{:else}
		<ol class="rail-list">
			{#each citations as cite, i (i + (cite.RefID ?? cite.URL))}
				{@const href = hrefFor(cite)}
				{@const external = isExternal(cite)}
				<li class="rail-item-wrap">
					<div
						class="rail-item"
						class:is-active={i === activeIndex}
						role="button"
						tabindex="0"
						onclick={() => handleSelect(i)}
						onkeydown={(e) => handleKey(e, i)}
					>
						<span class="item-num">{pad2(i + 1)}</span>
						<div class="item-body">
							{#if href}
								<a
									{href}
									class="item-title"
									target={external ? "_blank" : undefined}
									rel={external ? "noopener noreferrer" : undefined}
									onclick={(e) => e.stopPropagation()}
								>
									{cite.Title || "Untitled source"}
								</a>
							{:else}
								<span class="item-title item-title-disabled">
									{cite.Title || "Untitled source"}
								</span>
							{/if}
							{#if cite.PublishedAt}
								<p class="item-dateline">{formatDateline(cite.PublishedAt)}</p>
							{/if}
							{#if href && external}
								<a
									{href}
									class="item-domain"
									target="_blank"
									rel="noopener noreferrer"
									onclick={(e) => e.stopPropagation()}
								>
									<span>{formatDomain(cite.URL)}</span>
									<ArrowUpRight size={11} strokeWidth={2} />
								</a>
							{/if}
						</div>
					</div>
				</li>
			{/each}
		</ol>
	{/if}
</aside>

<style>
	.citation-rail {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		padding: 1rem 1.25rem 2rem;
		border-left: 1px solid var(--surface-border, #c8c8c8);
		height: 100%;
		overflow-y: auto;
	}

	.rail-head {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.rail-title {
		font-family: var(--font-display, "Playfair Display", serif);
		font-size: 0.95rem;
		font-weight: 700;
		letter-spacing: -0.01em;
		color: var(--alt-charcoal, var(--text-primary, #1a1a1a));
		margin: 0;
	}

	.rail-rule {
		height: 1px;
		background: var(--surface-border, #c8c8c8);
	}

	.rail-empty {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.85rem;
		color: var(--text-muted, #999);
		margin: 0.75rem 0 0;
	}

	.rail-list {
		list-style: none;
		padding: 0;
		margin: 0;
		display: flex;
		flex-direction: column;
	}

	.rail-item-wrap + .rail-item-wrap {
		border-top: 1px solid var(--surface-border, #c8c8c8);
	}

	.rail-item {
		display: flex;
		gap: 0.7rem;
		padding: 0.7rem 0.5rem 0.7rem 0;
		cursor: pointer;
		border-left: 2px solid transparent;
		padding-left: 0.6rem;
		transition: background 120ms ease, border-color 120ms ease;
	}

	.rail-item:hover,
	.rail-item:focus-visible,
	.rail-item.is-active {
		background: var(--surface-hover, rgba(0, 0, 0, 0.04));
		border-left-color: var(--accent-primary, #2f4f4f);
		outline: none;
	}

	.item-num {
		font-family: var(--font-mono, "IBM Plex Mono", monospace);
		font-size: 0.7rem;
		font-weight: 600;
		color: var(--text-muted, #999);
		min-width: 1.8rem;
		padding-top: 0.05rem;
	}

	.item-body {
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
		flex: 1;
		min-width: 0;
	}

	.item-title {
		font-family: var(--font-display, "Playfair Display", serif);
		font-size: 0.95rem;
		font-weight: 700;
		line-height: 1.3;
		letter-spacing: -0.01em;
		color: var(--alt-charcoal, var(--text-primary, #1a1a1a));
		text-decoration: none;
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}

	.item-title:hover {
		text-decoration: underline;
		text-decoration-thickness: 1px;
		text-underline-offset: 2px;
	}

	.item-title-disabled {
		opacity: 0.5;
		cursor: default;
	}
	.item-title-disabled:hover {
		text-decoration: none;
	}

	.item-dateline {
		font-family: var(--font-mono, "IBM Plex Mono", monospace);
		font-size: 0.65rem;
		font-weight: 500;
		letter-spacing: 0.18em;
		text-transform: uppercase;
		color: var(--text-muted, #999);
		margin: 0.05rem 0 0;
	}

	.item-domain {
		font-family: var(--font-mono, "IBM Plex Mono", monospace);
		font-size: 0.7rem;
		color: var(--alt-slate, #666);
		text-decoration: none;
		display: inline-flex;
		align-items: center;
		gap: 0.2rem;
		margin-top: 0.05rem;
		max-width: 100%;
		overflow: hidden;
	}

	.item-domain > span {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.item-domain:hover {
		color: var(--accent-primary, #2f4f4f);
	}

	/* skeleton */
	.rail-item.is-skeleton {
		cursor: default;
	}
	.skeleton-line {
		height: 0.65rem;
		background: linear-gradient(
			90deg,
			var(--surface-bg, rgba(0, 0, 0, 0.05)) 0%,
			var(--surface-hover, rgba(0, 0, 0, 0.1)) 50%,
			var(--surface-bg, rgba(0, 0, 0, 0.05)) 100%
		);
		background-size: 200% 100%;
		animation: shimmer 1.4s infinite;
		border-radius: 1px;
	}
	.skeleton-line-title {
		width: 80%;
		height: 0.85rem;
		margin-bottom: 0.3rem;
	}
	.skeleton-line-meta {
		width: 50%;
	}
	@keyframes shimmer {
		from {
			background-position: 200% 0;
		}
		to {
			background-position: -200% 0;
		}
	}
</style>
