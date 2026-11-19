<script lang="ts">
import { Code, ConnectError } from "@connectrpc/connect";
import {
	getTheme,
	recordThemeOpened,
	type ThemePageData,
} from "#lib/connect/reading.js";
import { getReadingTransport } from "#lib/connect/reading-transport.js";
import { openReaderOverlay } from "#lib/reader/reader-overlay.svelte.js";
import { ReadingSurface } from "#lib/reading/reading-events.js";
import {
	formatCalendarDate,
	formatThemePageMeta,
	formatThemeVersionRow,
	parseVersionParam,
	resolveSourceMarkers,
	sourceTitleOf,
	type ThemeSourceMarker,
	toThemeReaderItems,
} from "#lib/reading/themes-format.js";
import { page } from "$app/state";

type LoadState = "loading" | "ready" | "not-found" | "error";

const themeId = $derived(page.params.themeId ?? "");
const requestedVersion = $derived(
	parseVersionParam(page.url.searchParams.get("version")),
);
const latestHref = $derived(`/knowledge/themes/${encodeURIComponent(themeId)}`);

let theme = $state<ThemePageData | null>(null);
let loadState = $state<LoadState>("loading");
let requestSeq = 0;
let lastRecordedKey: string | null = null;

async function loadThemePage(id: string, version: number | undefined) {
	const seq = ++requestSeq;
	loadState = "loading";
	theme = null;

	try {
		const data = await getTheme(getReadingTransport(), id, version);
		if (seq !== requestSeq) return;
		theme = data;
		loadState = "ready";

		const key = `${data.themeId}:${data.version}`;
		if (lastRecordedKey !== key) {
			lastRecordedKey = key;
			void recordThemeOpened(
				getReadingTransport(true),
				data.themeId,
				data.version,
			);
		}
	} catch (err) {
		if (seq !== requestSeq) return;
		if (err instanceof ConnectError && err.code === Code.NotFound) {
			loadState = "not-found";
		} else {
			loadState = "error";
		}
	}
}

$effect(() => {
	if (!themeId) {
		loadState = "not-found";
		theme = null;
		return;
	}
	void loadThemePage(themeId, requestedVersion);
});

function openSourceAt(sourceIndex: number) {
	if (!theme || theme.sources.length === 0) return;
	openReaderOverlay({
		surface: ReadingSurface.OTHER,
		items: toThemeReaderItems(theme.sources),
		currentIndex: sourceIndex,
	});
}
</script>

<svelte:head>
	<title>{theme ? `${theme.title} - Alt` : "Theme - Alt"}</title>
</svelte:head>

{#snippet entryBody(text: string, markers: ThemeSourceMarker[])}
	<p class="entry-sentence">{text}</p>
	{#if markers.length > 0}
		<div class="marker-row" data-testid="theme-marker-row">
			{#each markers as marker (marker.number)}
				{@const title = sourceTitleOf(marker.source.title, marker.source.url)}
				<button
					type="button"
					class="source-marker"
					aria-label="Open source {marker.number}: {title}"
					onclick={() => openSourceAt(marker.sourceIndex)}
				>[{marker.number}]</button>
			{/each}
		</div>
	{/if}
{/snippet}

<div class="theme-page">
	{#if loadState !== "not-found"}
		<nav class="back-nav" aria-label="Breadcrumb">
			<a href="/knowledge" class="quiet-link" data-testid="theme-back-link">
				Themes
			</a>
		</nav>
	{/if}

	{#if loadState === "loading"}
		<div class="state-block state-loading" data-testid="theme-loading">
			<span class="loading-dot" aria-hidden="true"></span>
			<span class="state-text">Loading theme...</span>
		</div>
	{:else if loadState === "not-found"}
		<div class="state-block" data-testid="theme-not-found">
			<p class="empty-sentence">This theme does not exist.</p>
			<a href="/knowledge" class="quiet-link" data-testid="theme-back-link">
				Themes
			</a>
		</div>
	{:else if loadState === "error"}
		<div class="state-block state-error" role="alert" data-testid="theme-error">
			<p class="error-text">Could not load theme.</p>
			<button
				type="button"
				class="quiet-btn"
				data-testid="theme-retry"
				onclick={() => void loadThemePage(themeId, requestedVersion)}
			>
				Retry
			</button>
		</div>
	{:else if theme}
		<header class="theme-header">
			<h1 class="theme-title">{theme.title}</h1>
			<p class="theme-meta" data-testid="theme-meta">
				{formatThemePageMeta(
					theme.version,
					theme.latestVersion,
					theme.createdAt,
					theme.sources.length,
				)}
			</p>
			{#if theme.version < theme.latestVersion}
				<p class="older-notice" data-testid="theme-older-notice">
					<span>You are reading an older version.</span>
					<a href={latestHref} class="inline-mono-link">Latest</a>
				</p>
			{/if}
		</header>

		{#if theme.keyPoints.length > 0}
			<section
				class="theme-section"
				aria-labelledby="heading-key-points"
				data-testid="theme-section-key-points"
			>
				<div class="section-head">
					<h2 id="heading-key-points" class="section-heading">Key points</h2>
				</div>
				<ul class="sentence-list">
					{#each theme.keyPoints as item, idx (idx)}
						{@const markers = resolveSourceMarkers(
							item.sourceArticleIds,
							theme.sources,
						)}
						<li
							class="entry-row"
							class:entry-row--has-markers={markers.length > 0}
						>
							{@render entryBody(item.text, markers)}
						</li>
					{/each}
				</ul>
			</section>
		{/if}

		{#if theme.timeline.length > 0}
			<section
				class="theme-section"
				aria-labelledby="heading-timeline"
				data-testid="theme-section-timeline"
			>
				<div class="section-head">
					<h2 id="heading-timeline" class="section-heading">Timeline</h2>
				</div>
				<ul class="timeline-list">
					{#each theme.timeline as entry, idx (idx)}
						{@const markers = resolveSourceMarkers(
							entry.sourceArticleIds,
							theme.sources,
						)}
						{@const formattedDate = formatCalendarDate(entry.date)}
						<li
							class="entry-row timeline-row"
							class:entry-row--has-markers={markers.length > 0}
						>
							{#if formattedDate}
								<span class="timeline-date">{formattedDate}</span>
							{/if}
							<div class="timeline-body">
								{@render entryBody(entry.text, markers)}
							</div>
						</li>
					{/each}
				</ul>
			</section>
		{/if}

		{#if theme.whatChanged.length > 0}
			<section
				class="theme-section"
				aria-labelledby="heading-what-changed"
				data-testid="theme-section-what-changed"
			>
				<div class="section-head">
					<h2 id="heading-what-changed" class="section-heading">What changed</h2>
				</div>
				<ul class="sentence-list">
					{#each theme.whatChanged as item, idx (idx)}
						{@const markers = resolveSourceMarkers(
							item.sourceArticleIds,
							theme.sources,
						)}
						<li
							class="entry-row"
							class:entry-row--has-markers={markers.length > 0}
						>
							{@render entryBody(item.text, markers)}
						</li>
					{/each}
				</ul>
			</section>
		{/if}

		{#if theme.openQuestions.length > 0}
			<section
				class="theme-section"
				aria-labelledby="heading-open-questions"
				data-testid="theme-section-open-questions"
			>
				<div class="section-head">
					<h2 id="heading-open-questions" class="section-heading">
						Open questions
					</h2>
				</div>
				<ul class="sentence-list">
					{#each theme.openQuestions as item, idx (idx)}
						{@const markers = resolveSourceMarkers(
							item.sourceArticleIds,
							theme.sources,
						)}
						<li
							class="entry-row"
							class:entry-row--has-markers={markers.length > 0}
						>
							{@render entryBody(item.text, markers)}
						</li>
					{/each}
				</ul>
			</section>
		{/if}

		{#if theme.sources.length > 0}
			<section
				class="theme-section"
				aria-labelledby="heading-sources"
				data-testid="theme-section-sources"
			>
				<div class="section-head">
					<h2 id="heading-sources" class="section-heading">Sources</h2>
				</div>
				<ol class="sources-list">
					{#each theme.sources as source, index (source.articleId)}
						<li class="source-list-item">
							<button
								type="button"
								class="source-row"
								data-testid="theme-source-row"
								onclick={() => openSourceAt(index)}
							>
								<span class="source-index">[{index + 1}]</span>
								<span class="source-title">
									{sourceTitleOf(source.title, source.url)}
								</span>
							</button>
						</li>
					{/each}
				</ol>
			</section>
		{/if}

		{#if theme.versions.length > 1}
			<section
				class="theme-section"
				aria-labelledby="heading-versions"
				data-testid="theme-section-versions"
			>
				<div class="section-head">
					<h2 id="heading-versions" class="section-heading">Versions</h2>
				</div>
				<ul class="versions-list">
					{#each theme.versions as ver (ver.version)}
						{@const label = formatThemeVersionRow(
							ver.version,
							ver.createdAt,
							ver.articleCount,
						)}
						<li class="version-row">
							{#if ver.version === theme.version}
								<span class="version-current">{label}</span>
							{:else}
								<a href={`${latestHref}?version=${ver.version}`} class="version-link">
									{label}
								</a>
							{/if}
						</li>
					{/each}
				</ul>
			</section>
		{/if}
	{/if}
</div>

<style>
	.theme-page {
		max-width: 42rem;
		margin: 0 auto;
		padding: 0.75rem 1.25rem 2.5rem;
		background: var(--surface-bg);
		color: var(--alt-charcoal);
		font-family: var(--font-body);
		box-sizing: border-box;
	}

	.back-nav {
		margin-bottom: 0.25rem;
	}

	.quiet-link,
	.quiet-btn {
		display: inline-flex;
		align-items: center;
		min-height: 44px;
		min-width: 44px;
		padding: 0;
		border: none;
		background: transparent;
		font-family: var(--font-mono);
		font-size: 0.75rem;
		color: var(--alt-slate);
		text-decoration: underline;
		text-underline-offset: 0.2em;
		cursor: pointer;
		touch-action: manipulation;
	}

	.quiet-btn {
		color: var(--alt-charcoal);
	}

	.quiet-link:active,
	.quiet-btn:active {
		color: var(--alt-charcoal);
	}

	.theme-header {
		display: flex;
		flex-direction: column;
		gap: 0.35rem;
		padding-bottom: 0.75rem;
		border-bottom: 1px solid var(--surface-border);
	}

	.theme-title {
		font-family: var(--font-display);
		font-size: 1.5rem;
		font-weight: 700;
		line-height: 1.2;
		color: var(--alt-charcoal);
		margin: 0;
		overflow-wrap: anywhere;
	}

	.theme-meta {
		margin: 0;
		font-family: var(--font-mono);
		font-size: 0.72rem;
		line-height: 1.4;
		color: var(--alt-ash);
		overflow-wrap: anywhere;
	}

	.older-notice {
		margin: 0.25rem 0 0;
		padding: 0.35rem 0.75rem;
		border-left: 3px solid var(--alt-charcoal);
		background: var(--surface-2);
		font-size: 0.85rem;
		line-height: 1.4;
		color: var(--alt-charcoal);
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.35rem;
	}

	.inline-mono-link {
		display: inline-flex;
		align-items: center;
		min-height: 44px;
		min-width: 44px;
		padding: 0 0.25rem;
		font-family: var(--font-mono);
		font-size: 0.75rem;
		color: var(--alt-primary);
		text-decoration: underline;
		text-underline-offset: 0.2em;
		touch-action: manipulation;
	}

	.theme-section {
		margin-top: 1.35rem;
	}

	.section-head {
		padding-bottom: 0.5rem;
		border-bottom: 1px solid var(--alt-charcoal);
	}

	.section-heading {
		font-family: var(--font-body);
		font-size: 0.78rem;
		font-weight: 600;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--alt-charcoal);
		margin: 0;
	}

	.sentence-list,
	.timeline-list,
	.versions-list {
		list-style: none;
		margin: 0;
		padding: 0;
	}

	.sources-list {
		list-style: none;
		margin: 0;
		padding: 0;
	}

	.entry-row {
		padding-top: 0.9rem;
		padding-bottom: 0.9rem;
		border-bottom: 1px solid var(--surface-border);
	}

	.entry-row--has-markers {
		padding-bottom: 0;
	}

	.entry-row:last-child {
		border-bottom: none;
	}

	.entry-sentence {
		margin: 0;
		font-family: var(--font-body);
		font-size: 1rem;
		line-height: 1.55;
		color: var(--alt-charcoal);
		overflow-wrap: anywhere;
	}

	.marker-row {
		display: flex;
		flex-wrap: wrap;
		gap: 0;
	}

	.source-marker {
		display: inline-flex;
		align-items: center;
		justify-content: flex-start;
		min-width: 44px;
		min-height: 44px;
		padding: 0;
		border: none;
		background: transparent;
		font-family: var(--font-mono);
		font-size: 0.72rem;
		color: var(--alt-slate);
		text-decoration: underline;
		text-underline-offset: 0.2em;
		cursor: pointer;
		touch-action: manipulation;
	}

	.source-marker:active {
		color: var(--alt-charcoal);
	}

	.timeline-row {
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
	}

	.timeline-date {
		font-family: var(--font-mono);
		font-size: 0.72rem;
		line-height: 1.55;
		color: var(--alt-ash);
		flex-shrink: 0;
	}

	.timeline-body {
		min-width: 0;
	}

	.source-list-item {
		border-bottom: 1px solid var(--surface-border);
	}

	.source-row {
		width: 100%;
		min-height: 44px;
		padding: 0.65rem 0;
		border: none;
		background: transparent;
		text-align: left;
		display: flex;
		align-items: baseline;
		gap: 0.5rem;
		cursor: pointer;
		touch-action: manipulation;
	}

	.source-row:active .source-title {
		text-decoration: underline;
	}

	.source-index {
		flex-shrink: 0;
		font-family: var(--font-mono);
		font-size: 0.75rem;
		color: var(--alt-ash);
	}

	.source-title {
		min-width: 0;
		font-family: var(--font-body);
		font-size: 0.95rem;
		line-height: 1.4;
		color: var(--alt-primary);
		overflow-wrap: anywhere;
	}

	.version-row {
		border-bottom: 1px solid var(--surface-border);
	}

	.version-current {
		display: inline-flex;
		align-items: center;
		min-height: 44px;
		font-family: var(--font-mono);
		font-size: 0.75rem;
		color: var(--alt-charcoal);
	}

	.version-link {
		display: inline-flex;
		align-items: center;
		min-height: 44px;
		min-width: 44px;
		font-family: var(--font-mono);
		font-size: 0.75rem;
		color: var(--alt-primary);
		text-decoration: underline;
		text-underline-offset: 0.2em;
		touch-action: manipulation;
	}

	.state-block {
		padding: 1.5rem 0;
		border-bottom: 1px solid var(--surface-border);
	}

	.state-loading {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.loading-dot {
		width: 8px;
		height: 8px;
		background: var(--alt-ash);
		animation: pulse 1.2s ease-in-out infinite;
	}

	.state-text {
		font-size: 0.85rem;
		color: var(--alt-slate);
	}

	.state-error {
		border-left: 3px solid var(--alt-error);
		padding-left: 0.75rem;
	}

	.error-text {
		margin: 0 0 0.5rem;
		font-size: 0.85rem;
		color: var(--alt-error);
	}

	.empty-sentence {
		margin: 0 0 0.5rem;
		font-size: 0.92rem;
		color: var(--alt-slate);
	}

	@media (min-width: 30rem) {
		.timeline-row {
			display: grid;
			grid-template-columns: 7rem minmax(0, 1fr);
			column-gap: 0.75rem;
			align-items: baseline;
		}

		.timeline-date {
			grid-column: 1;
		}

		.timeline-body {
			grid-column: 2;
		}
	}

	@keyframes pulse {
		0%,
		100% {
			opacity: 0.3;
		}
		50% {
			opacity: 1;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.loading-dot {
			animation: none;
			opacity: 0.6;
		}
	}
</style>
