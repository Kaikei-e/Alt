<script lang="ts">
import { parseMarkdown } from "$lib/utils/simpleMarkdown";
import augurAvatar from "$lib/assets/augur-chat.webp";

type Citation = {
	URL: string;
	Title: string;
	PublishedAt?: string;
	Score?: number;
};

type Props = {
	message: string;
	role: "user" | "assistant";
	timestamp?: string;
	citations?: Citation[];
	index?: number;
};

let { message, role, timestamp, citations, index = 0 }: Props = $props();

let isUser = $derived(role === "user");
</script>

<article class="thread-entry" data-role={role} style="--stagger: {index}">
	{#if isUser}
		<h3 class="entry-question">{message}</h3>
	{:else}
		<div class="entry-byline">
			<img src={augurAvatar} alt="Augur" class="byline-avatar" />
			<span class="byline-name">Augur</span>
			{#if timestamp}
				<span class="byline-time">{timestamp}</span>
			{/if}
		</div>
		<div class="entry-prose">
			{@html parseMarkdown(message)}
		</div>
		{#if citations && citations.length > 0}
			<footer class="entry-sources">
				<h4 class="sources-heading">Sources</h4>
				<ol class="sources-list">
					{#each citations as cite, i}
						<li class="source-item">
							<span class="source-id">[{i + 1}]</span>
							<a
								href={cite.URL}
								target="_blank"
								rel="noopener noreferrer"
								class="source-title"
							>
								{cite.Title || "Untitled Source"}
							</a>
						</li>
					{/each}
				</ol>
			</footer>
		{/if}
	{/if}

	<div class="entry-rule"></div>
</article>

<style>
	.thread-entry {
		padding: 1rem 0;
		opacity: 0;
		animation: entry-in 0.3s ease forwards;
		animation-delay: calc(var(--stagger) * 60ms);
	}
	@keyframes entry-in { to { opacity: 1; } }

	/* User question — the question speaks for itself */
	.entry-question {
		font-family: var(--font-display, "Playfair Display", serif);
		font-size: 1.15rem; font-weight: 700; line-height: 1.3;
		color: var(--alt-charcoal, #1a1a1a);
		margin: 0;
	}

	/* Augur byline: avatar + name + time */
	.entry-byline {
		display: flex; align-items: center; gap: 0.4rem;
		margin-bottom: 0.4rem;
	}
	.byline-avatar {
		width: 24px; height: 24px;
		object-fit: cover;
		border: 1px solid var(--surface-border, #c8c8c8);
	}
	.byline-name {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.65rem; font-weight: 600;
		letter-spacing: 0.08em; text-transform: uppercase;
		color: var(--alt-ash, #999);
	}
	.byline-time {
		font-family: var(--font-mono, "IBM Plex Mono", monospace);
		font-size: 0.6rem; color: var(--alt-ash, #999);
	}

	/* Assistant prose */
	.entry-prose {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.95rem; line-height: 1.72;
		color: var(--alt-charcoal, #1a1a1a);
		max-width: 65ch;
	}
	.entry-prose :global(h1) {
		font-family: var(--font-display, "Playfair Display", serif);
		font-size: 1.3rem; font-weight: 700; margin: 1.5rem 0 0.5rem; line-height: 1.25;
	}
	.entry-prose :global(h2) {
		font-family: var(--font-display, "Playfair Display", serif);
		font-size: 1.1rem; font-weight: 700; margin: 1.25rem 0 0.4rem; line-height: 1.3;
	}
	.entry-prose :global(h3) {
		font-family: var(--font-display, "Playfair Display", serif);
		font-size: 0.95rem; font-weight: 700; margin: 1rem 0 0.3rem; line-height: 1.35;
	}
	.entry-prose :global(p) { margin: 0 0 0.75rem; line-height: 1.72; }
	.entry-prose :global(ul),
	.entry-prose :global(ol) { margin: 0.5rem 0 0.75rem; padding-left: 1.5rem; }
	.entry-prose :global(ul) { list-style-type: disc; }
	.entry-prose :global(ol) { list-style-type: decimal; }
	.entry-prose :global(li) { margin-bottom: 0.25rem; line-height: 1.6; }
	.entry-prose :global(blockquote) {
		border-left: 2px solid var(--alt-charcoal, #1a1a1a); padding-left: 0.75rem;
		margin: 0.75rem 0; font-style: italic; color: var(--alt-slate, #666);
	}
	.entry-prose :global(a) {
		color: var(--alt-primary, #2f4f4f); text-decoration: underline;
		text-decoration-thickness: 1px; text-underline-offset: 2px; transition: color 0.15s;
	}
	.entry-prose :global(a:hover) { color: var(--alt-charcoal, #1a1a1a); }
	.entry-prose :global(hr) { border: none; border-top: 1px solid var(--surface-border, #c8c8c8); margin: 1.25rem 0; }
	.entry-prose :global(pre) {
		background: var(--surface-2, #f5f4f1); padding: 0.75rem; overflow-x: auto;
		margin: 0.75rem 0; font-size: 0.85rem; line-height: 1.5;
	}
	.entry-prose :global(code) { font-family: var(--font-mono, "IBM Plex Mono", monospace); font-size: 0.85em; }
	.entry-prose :global(strong) { font-weight: 700; }

	/* Sources / citations — only shown on narrower viewports.
	   At ≥1280px the AugurChat right-column rail takes over as the canonical
	   citation surface, so this footer collapses to avoid duplication. */
	.entry-sources {
		margin-top: 1rem; padding-top: 0.6rem;
		border-top: 1px solid var(--surface-border, #c8c8c8);
		max-width: 65ch;
	}
	@media (min-width: 1280px) {
		.entry-sources { display: none; }
	}
	.sources-heading {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.6rem; font-weight: 700; letter-spacing: 0.12em;
		text-transform: uppercase; color: var(--alt-ash, #999);
		margin: 0 0 0.4rem;
	}
	.sources-list {
		list-style: none; padding: 0; margin: 0;
		display: flex; flex-direction: column; gap: 0.3rem;
	}
	.source-item {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.75rem; line-height: 1.5; color: var(--alt-slate, #666);
		display: flex; gap: 0.3rem; align-items: baseline;
	}
	.source-id {
		font-family: var(--font-mono, "IBM Plex Mono", monospace);
		font-size: 0.65rem; font-weight: 600; color: var(--alt-charcoal, #1a1a1a);
		flex-shrink: 0;
	}
	.source-title {
		color: var(--alt-primary, #2f4f4f); text-decoration: underline;
		text-decoration-thickness: 1px; text-underline-offset: 2px;
		transition: color 0.15s;
	}
	.source-title:hover { color: var(--alt-charcoal, #1a1a1a); }

	/* Bottom rule separator */
	.entry-rule {
		height: 1px; background: var(--surface-border, #c8c8c8);
		margin-top: 1rem;
	}

	@media (prefers-reduced-motion: reduce) {
		.thread-entry { animation: none; opacity: 1; }
	}
</style>
