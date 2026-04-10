<script lang="ts">
import { onMount } from "svelte";
import { useViewport } from "$lib/stores/viewport.svelte";
import { useFeedStats } from "$lib/hooks/useFeedStats.svelte";
import { useTrendStats } from "$lib/hooks/useTrendStats.svelte";
import TimeWindowSelector from "$lib/components/desktop/stats/TimeWindowSelector.svelte";
import type { TimeWindow } from "$lib/schema/stats";

// Mobile deps
import {
	getDetailedFeedStatsClient,
	getUnreadCountClient,
} from "$lib/api/client/feeds";
import type { DetailedFeedStatsSummary } from "$lib/schema/stats";

const { isDesktop } = useViewport();

// Lazy load chart.js (heavy dependency) - only loaded when stats page is visited on desktop
const TrendChartPromise = import(
	"$lib/components/desktop/stats/TrendChart.svelte"
);

// Shared hook
const stats = useFeedStats();

// Desktop-specific
const trendStats = useTrendStats();

let feedAmount = $derived(stats.feedAmount);
let totalArticlesAmount = $derived(stats.totalArticlesAmount);
let unsummarizedArticlesAmount = $derived(stats.unsummarizedArticlesAmount);

// Page reveal
let revealed = $state(false);

const dateStr = new Date().toLocaleDateString("en-US", {
	weekday: "long",
	year: "numeric",
	month: "long",
	day: "numeric",
});

// Mobile-specific state
let mobileStats: DetailedFeedStatsSummary | null = $state(null);
let unreadCount = $state(0);
let mobileLoading = $state(true);
let mobileError: string | null = $state(null);

let displayFeedAmount = $state(0);
let displayTotalArticles = $state(0);
let displayUnsummarized = $state(0);

function formatNumber(num: number): string {
	return new Intl.NumberFormat().format(num);
}

onMount(async () => {
	requestAnimationFrame(() => {
		revealed = true;
	});

	if (isDesktop) {
		trendStats.fetchData("24h");
	} else {
		try {
			const [statsData, unreadData] = await Promise.all([
				getDetailedFeedStatsClient(),
				getUnreadCountClient(),
			]);
			mobileStats = statsData;

			displayFeedAmount = statsData.feed_amount.amount;
			displayTotalArticles = statsData.total_articles.amount;
			displayUnsummarized = statsData.unsummarized_articles.amount;

			unreadCount = unreadData.count;
		} catch (e) {
			console.error("Failed to fetch stats", e);
			mobileError = "Failed to load statistics";
		} finally {
			mobileLoading = false;
		}
	}
});

// Mobile: synchronize SSE updates with display values
$effect(() => {
	if (!isDesktop && stats.isConnected) {
		if (stats.feedAmount > 0) {
			displayFeedAmount = stats.feedAmount;
		}
		if (stats.totalArticlesAmount > 0) {
			displayTotalArticles = stats.totalArticlesAmount;
		}
		if (stats.unsummarizedArticlesAmount > 0) {
			displayUnsummarized = stats.unsummarizedArticlesAmount;
		}
	}
});
</script>

<svelte:head>
	<title>Statistics - Alt</title>
</svelte:head>

{#if isDesktop}
	<div class="ledger-page" class:revealed>
		<!-- Editorial Header -->
		<header class="ledger-header">
			<span class="ledger-date">{dateStr}</span>
			<h1 class="ledger-title">Circulation Ledger</h1>
			<div class="ledger-rule" aria-hidden="true"></div>
		</header>

		<!-- Figures Bar -->
		<div class="section-reveal" style="--delay: 1;">
			<div class="figures-bar">
				<div class="figure-group">
					<span class="figure-label">FEEDS</span>
					<span class="figure-value">{feedAmount.toLocaleString()}</span>
				</div>

				<div class="figure-separator" aria-hidden="true"></div>

				<div class="figure-group">
					<span class="figure-label">ARTICLES</span>
					<span class="figure-value"
						>{totalArticlesAmount.toLocaleString()}</span
					>
				</div>

				<div class="figure-separator" aria-hidden="true"></div>

				<div class="figure-group">
					<span class="figure-label">UNSUMMARIZED</span>
					<span class="figure-value"
						>{unsummarizedArticlesAmount.toLocaleString()}</span
					>
				</div>

				<div class="status-group">
					<span
						class="status-dot"
						class:status-dot--live={stats.isConnected}
						class:status-dot--offline={!stats.isConnected}
					></span>
					<span class="status-label"
						>{stats.isConnected ? "Live" : "Offline"}</span
					>
					{#if !stats.isConnected}
						<button
							class="reconnect-btn"
							onclick={() => stats.reconnect()}
						>
							RECONNECT
						</button>
					{/if}
				</div>
			</div>
			<div class="ledger-rule" aria-hidden="true"></div>
		</div>

		<!-- Activity Log Section -->
		<div class="section-reveal" style="--delay: 2;">
			<div class="activity-log-header">
				<h2 class="activity-log-label">Activity Log</h2>
				<TimeWindowSelector
					selected={trendStats.currentWindow}
					onchange={(window: TimeWindow) => trendStats.setWindow(window)}
				/>
			</div>

			{#if trendStats.error}
				<div class="trend-error">
					<span class="trend-error-text">{trendStats.error}</span>
				</div>
			{/if}

			<div class="charts-grid">
				{#await TrendChartPromise then TrendChart}
					<TrendChart.default
						title="Articles"
						dataPoints={trendStats.data?.data_points ?? []}
						dataKey="articles"
						loading={trendStats.loading}
					/>
					<TrendChart.default
						title="Summarized"
						dataPoints={trendStats.data?.data_points ?? []}
						dataKey="summarized"
						loading={trendStats.loading}
					/>
					<TrendChart.default
						title="Feed Activity"
						dataPoints={trendStats.data?.data_points ?? []}
						dataKey="feed_activity"
						loading={trendStats.loading}
					/>
				{/await}
			</div>
		</div>
	</div>
{:else}
	<!-- Mobile -->
	<div class="ledger-mobile">
		<header class="mobile-ledger-header">
			<span class="ledger-date">{dateStr}</span>
			<h1 class="ledger-title-mobile">Circulation Ledger</h1>
			<div class="ledger-rule" aria-hidden="true"></div>
		</header>

		<div class="mobile-content">
			<!-- Status -->
			<div class="mobile-status-row">
				<span
					class="status-dot"
					class:status-dot--live={stats.isConnected}
					class:status-dot--offline={!stats.isConnected}
				></span>
				<span class="status-label"
					>{stats.isConnected ? "Live" : "Offline"}</span
				>
			</div>

			{#if mobileLoading}
				<div class="mobile-loading">
					<span class="loading-pulse"></span>
					<span class="loading-text">Loading&hellip;</span>
				</div>
			{:else if mobileError}
				<div class="mobile-error">
					<span class="error-label">Error</span>
					<span class="error-text">{mobileError}</span>
				</div>
			{:else}
				<div class="ledger-rows">
					<div class="ledger-row" data-testid="stat-total-feeds">
						<span class="row-label">Total Feeds</span>
						<span class="row-value">{formatNumber(displayFeedAmount)}</span>
					</div>
					<div class="ledger-rule" aria-hidden="true"></div>

					<div class="ledger-row" data-testid="stat-total-articles">
						<span class="row-label">Total Articles</span>
						<span class="row-value">{formatNumber(displayTotalArticles)}</span>
					</div>
					<div class="ledger-rule" aria-hidden="true"></div>

					<div class="ledger-row" data-testid="stat-unsummarized">
						<span class="row-label">Unsummarized</span>
						<span class="row-value">{formatNumber(displayUnsummarized)}</span>
					</div>
					<div class="ledger-rule" aria-hidden="true"></div>

					<div class="ledger-row" data-testid="stat-unread-count">
						<span class="row-label">Today's Unread</span>
						<span class="row-value">{formatNumber(unreadCount)}</span>
					</div>
				</div>
			{/if}
		</div>
	</div>
{/if}

<style>
	/* ── Page reveal ── */
	.ledger-page {
		max-width: 1400px;
		opacity: 0;
		transform: translateY(6px);
		transition:
			opacity 0.4s ease,
			transform 0.4s ease;
	}

	.ledger-page.revealed {
		opacity: 1;
		transform: translateY(0);
	}

	/* ── Header ── */
	.ledger-header {
		padding: 1.5rem 0 0;
	}

	.mobile-ledger-header {
		padding: 1rem 1.25rem 0;
	}

	.ledger-date {
		font-family: var(--font-mono);
		font-size: 0.7rem;
		color: var(--alt-ash);
		letter-spacing: 0.06em;
	}

	.ledger-title {
		font-family: var(--font-display);
		font-size: 1.6rem;
		font-weight: 800;
		color: var(--alt-charcoal);
		letter-spacing: -0.01em;
		margin: 0.15rem 0 0;
		line-height: 1.2;
	}

	.ledger-title-mobile {
		font-family: var(--font-display);
		font-size: 1.3rem;
		font-weight: 700;
		color: var(--alt-charcoal);
		margin: 0.1rem 0 0;
		line-height: 1.2;
	}

	.ledger-rule {
		height: 1px;
		background: var(--surface-border);
		margin-top: 0.75rem;
	}

	/* ── Figures bar ── */
	.figures-bar {
		display: flex;
		align-items: baseline;
		gap: 1.5rem;
		padding: 0.75rem 0;
	}

	.figure-group {
		display: flex;
		align-items: baseline;
		gap: 0.4rem;
	}

	.figure-label {
		font-family: var(--font-mono);
		font-size: 0.65rem;
		font-weight: 600;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--alt-ash);
	}

	.figure-value {
		font-family: var(--font-mono);
		font-size: 1.1rem;
		font-weight: 600;
		color: var(--alt-charcoal);
		font-variant-numeric: tabular-nums;
	}

	.figure-separator {
		width: 1px;
		height: 1.2rem;
		background: var(--surface-border);
		flex-shrink: 0;
	}

	/* ── Status ── */
	.status-group {
		display: flex;
		align-items: center;
		gap: 0.35rem;
		margin-left: auto;
	}

	.status-dot {
		width: 6px;
		height: 6px;
		border-radius: 50%;
		flex-shrink: 0;
	}

	.status-dot--live {
		background: var(--alt-sage);
		animation: pulse 1.2s ease-in-out infinite;
	}

	.status-dot--offline {
		background: var(--alt-ash);
		animation: none;
	}

	.status-label {
		font-family: var(--font-mono);
		font-size: 0.65rem;
		color: var(--alt-ash);
		letter-spacing: 0.04em;
	}

	.reconnect-btn {
		border: 1.5px solid var(--alt-charcoal);
		background: transparent;
		color: var(--alt-charcoal);
		font-family: var(--font-mono);
		font-size: 0.65rem;
		font-weight: 600;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		padding: 0.25rem 0.6rem;
		cursor: pointer;
		margin-left: 0.5rem;
		transition:
			background 0.15s,
			color 0.15s;
	}

	.reconnect-btn:hover {
		background: var(--alt-charcoal);
		color: var(--surface-bg);
	}

	/* ── Activity Log ── */
	.activity-log-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 1rem 0 0.75rem;
	}

	.activity-log-label {
		font-family: var(--font-mono);
		font-size: 0.65rem;
		font-weight: 600;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--alt-ash);
		margin: 0;
	}

	.charts-grid {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: 1rem;
	}

	.trend-error {
		border: 1px solid var(--alt-terracotta);
		padding: 0.75rem 1rem;
		margin-bottom: 1rem;
	}

	.trend-error-text {
		font-family: var(--font-body);
		font-size: 0.85rem;
		color: var(--alt-terracotta);
	}

	/* ── Section reveal animation ── */
	.section-reveal {
		opacity: 0;
		transform: translateY(6px);
		animation: reveal 0.4s ease forwards;
		animation-delay: calc(var(--delay) * 100ms);
	}

	/* ── Mobile ── */
	.ledger-mobile {
		height: 100vh;
		height: 100dvh;
		overflow-y: auto;
		display: flex;
		flex-direction: column;
		background: var(--surface-bg);
	}

	.mobile-content {
		padding: 1rem 1.25rem;
		flex: 1;
	}

	.mobile-status-row {
		display: flex;
		align-items: center;
		gap: 0.35rem;
		margin-bottom: 1rem;
	}

	.mobile-loading {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		padding: 4rem 0;
		gap: 0.75rem;
	}

	.loading-pulse {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--alt-ash);
		animation: pulse 1.2s ease-in-out infinite;
	}

	.loading-text {
		font-family: var(--font-body);
		font-size: 0.85rem;
		font-style: italic;
		color: var(--alt-ash);
	}

	.mobile-error {
		border: 1px solid var(--alt-terracotta);
		padding: 1rem;
		text-align: center;
	}

	.error-label {
		font-family: var(--font-mono);
		font-size: 0.65rem;
		font-weight: 600;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--alt-terracotta);
		display: block;
		margin-bottom: 0.25rem;
	}

	.error-text {
		font-family: var(--font-body);
		font-size: 0.85rem;
		color: var(--alt-slate);
	}

	/* ── Ledger rows (mobile figures) ── */
	.ledger-rows {
		padding-top: 0.5rem;
	}

	.ledger-row {
		display: flex;
		justify-content: space-between;
		align-items: baseline;
		padding: 0.75rem 0;
	}

	.row-label {
		font-family: var(--font-mono);
		font-size: 0.65rem;
		font-weight: 600;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--alt-ash);
	}

	.row-value {
		font-family: var(--font-mono);
		font-size: 1.1rem;
		font-weight: 600;
		color: var(--alt-charcoal);
		font-variant-numeric: tabular-nums;
	}

	/* ── Keyframes ── */
	@keyframes reveal {
		to {
			opacity: 1;
			transform: translateY(0);
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

	/* ── Reduced motion ── */
	@media (prefers-reduced-motion: reduce) {
		.ledger-page {
			opacity: 1;
			transform: none;
			transition: none;
		}

		.section-reveal {
			animation: none;
			opacity: 1;
			transform: none;
		}

		.status-dot--live {
			animation: none;
			opacity: 1;
		}

		.loading-pulse {
			animation: none;
			opacity: 1;
		}
	}
</style>
