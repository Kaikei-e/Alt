<script lang="ts">
import { onMount } from "svelte";
import { listThemes, type ThemeSummaryItem } from "#lib/connect/reading.js";
import { getReadingTransport } from "#lib/connect/reading-transport.js";
import {
	formatThemeListMeta,
	formatThemeStatusLabel,
} from "#lib/reading/themes-format.js";

let themes = $state<ThemeSummaryItem[]>([]);
let isLoading = $state(true);
let loadError = $state(false);
let requestSeq = 0;

async function loadThemes() {
	const seq = ++requestSeq;
	isLoading = true;
	loadError = false;
	try {
		const result = await listThemes(getReadingTransport());
		if (seq !== requestSeq) return;
		themes = result;
	} catch {
		if (seq !== requestSeq) return;
		loadError = true;
	} finally {
		if (seq === requestSeq) {
			isLoading = false;
		}
	}
}

onMount(() => {
	void loadThemes();
});
</script>

<svelte:head>
	<title>Themes - Alt</title>
</svelte:head>

<div class="themes-page">
	<header class="themes-header">
		<h1 class="themes-title">Themes</h1>
		<p class="themes-help">Pages built only from articles you have read.</p>
		<div class="themes-rule" aria-hidden="true"></div>
	</header>

	<main class="themes-content">
		{#if isLoading}
			<div class="state-block state-loading" data-testid="themes-loading">
				<span class="loading-dot" aria-hidden="true"></span>
				<span class="state-text">Loading themes...</span>
			</div>
		{:else if loadError}
			<div
				class="state-block state-error"
				role="alert"
				data-testid="themes-error"
			>
				<p class="error-text">Could not load themes.</p>
				<button
					type="button"
					class="retry-btn"
					data-testid="themes-retry"
					onclick={() => void loadThemes()}
				>
					Retry
				</button>
			</div>
		{:else if themes.length === 0}
			<div class="empty-block" data-testid="themes-empty">
				<p class="empty-sentence">
					No themes yet. A theme appears once you have read a few articles on
					the same subject.
				</p>
			</div>
		{:else}
			<ul class="themes-list" data-testid="themes-list">
				{#each themes as theme (theme.themeId)}
					{@const status = formatThemeStatusLabel(
						theme.lastOpenedVersion,
						theme.latestVersion,
					)}
					<li class="theme-row" data-testid="theme-row">
						<a
							href={`/knowledge/themes/${encodeURIComponent(theme.themeId)}`}
							class="theme-link"
						>
							<span class="theme-row-title">{theme.title}</span>
							<span class="theme-row-meta">
								{formatThemeListMeta(theme.articleCount, theme.updatedAt)}{#if status}<span class="theme-row-status">{" · "}{status}</span>{/if}
							</span>
						</a>
					</li>
				{/each}
			</ul>
		{/if}
	</main>
</div>

<style>
	.themes-page {
		max-width: 42rem;
		margin: 0 auto;
		padding: 1rem 1.25rem 2.5rem;
		background: var(--surface-bg);
		color: var(--alt-charcoal);
		font-family: var(--font-body);
		box-sizing: border-box;
	}

	.themes-header {
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
	}

	.themes-title {
		font-family: var(--font-display);
		font-size: 1.5rem;
		font-weight: 700;
		color: var(--alt-charcoal);
		margin: 0;
		line-height: 1.2;
	}

	.themes-help {
		margin: 0;
		font-size: 0.85rem;
		line-height: 1.45;
		color: var(--alt-slate);
	}

	.themes-rule {
		height: 1px;
		background: var(--alt-charcoal);
		margin-top: 0.55rem;
	}

	.themes-list {
		list-style: none;
		margin: 0;
		padding: 0;
	}

	.theme-row {
		border-bottom: 1px solid var(--surface-border);
	}

	.theme-link {
		display: flex;
		flex-direction: column;
		justify-content: center;
		gap: 0.25rem;
		min-height: 44px;
		padding: 0.85rem 0;
		text-decoration: none;
		color: inherit;
		touch-action: manipulation;
	}

	.theme-link:active .theme-row-title {
		text-decoration: underline;
	}

	.theme-row-title {
		font-family: var(--font-display);
		font-size: 1.05rem;
		font-weight: 600;
		line-height: 1.3;
		color: var(--alt-charcoal);
		overflow-wrap: anywhere;
	}

	.theme-row-meta {
		font-family: var(--font-mono);
		font-size: 0.72rem;
		line-height: 1.4;
		color: var(--alt-ash);
		overflow-wrap: anywhere;
	}

	.theme-row-status {
		color: var(--alt-primary);
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

	.retry-btn {
		display: inline-flex;
		align-items: center;
		min-height: 44px;
		min-width: 44px;
		padding: 0;
		border: none;
		background: transparent;
		font-family: var(--font-mono);
		font-size: 0.75rem;
		color: var(--alt-charcoal);
		text-decoration: underline;
		text-underline-offset: 0.2em;
		cursor: pointer;
		touch-action: manipulation;
	}

	.empty-block {
		padding: 1.75rem 0;
		border-bottom: 1px solid var(--surface-border);
	}

	.empty-sentence {
		margin: 0;
		font-size: 0.92rem;
		line-height: 1.5;
		color: var(--alt-slate);
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
