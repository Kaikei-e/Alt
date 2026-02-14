<script lang="ts">
import { onMount } from "svelte";
import { useViewport } from "$lib/stores/viewport.svelte";

// Desktop components & deps
import {
	BarChart3,
	TrendingUp,
	FileText,
	CheckCircle,
	BookOpen,
	Rss,
	Layers,
	RefreshCw,
} from "@lucide/svelte";
import PageHeader from "$lib/components/desktop/layout/PageHeader.svelte";
import { useFeedStats } from "$lib/hooks/useFeedStats.svelte";
import { useTrendStats } from "$lib/hooks/useTrendStats.svelte";
import TimeWindowSelector from "$lib/components/desktop/stats/TimeWindowSelector.svelte";

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
let summarizedArticles = $derived(
	totalArticlesAmount - unsummarizedArticlesAmount,
);
let connectionStatus = $derived(
	stats.isConnected ? "Connected" : "Disconnected",
);

let desktopStatCards = $derived([
	{
		label: "Feed Count",
		value: feedAmount,
		icon: FileText,
		color: "text-blue-600",
	},
	{
		label: "Total Articles",
		value: totalArticlesAmount,
		icon: BarChart3,
		color: "text-green-600",
	},
	{
		label: "Summarized",
		value: summarizedArticles,
		icon: CheckCircle,
		color: "text-purple-600",
	},
]);

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
	<PageHeader title="Statistics" description="Overview of your RSS feed analytics" />

	<!-- Stats cards -->
	<div class="grid grid-cols-1 md:grid-cols-3 gap-6 mb-8">
		{#each desktopStatCards as stat}
			<div class="border border-[var(--surface-border)] bg-white p-6">
				<div class="flex items-center justify-between mb-3">
					<h3 class="text-sm font-medium text-[var(--text-secondary)]">
						{stat.label}
					</h3>
					<stat.icon class="h-5 w-5 {stat.color}" />
				</div>
				<p class="text-3xl font-bold text-[var(--text-primary)]">
					{stat.value.toLocaleString()}
				</p>
			</div>
		{/each}
	</div>

	<!-- Connection status -->
	<div class="border border-[var(--surface-border)] bg-white p-6 mb-8">
		<div class="flex items-center justify-between">
			<div class="flex items-center gap-3">
				<div
					class="h-3 w-3 rounded-full {stats.isConnected ? 'bg-green-500' : 'bg-red-500'}"
				></div>
				<div>
					<h3 class="text-sm font-semibold text-[var(--text-primary)]">
						Real-time Connection
					</h3>
					<p class="text-xs text-[var(--text-secondary)]">{connectionStatus}</p>
				</div>
			</div>
				<div class="flex items-center gap-2">
				{#if !stats.isConnected}
					<button
						onclick={() => stats.reconnect()}
						class="inline-flex items-center gap-1.5 text-xs px-3 py-1 border border-[var(--surface-border)] rounded hover:bg-gray-50 text-[var(--text-secondary)] transition-colors"
					>
						<RefreshCw class="h-3 w-3" />
						Reconnect
					</button>
				{/if}
				<TrendingUp class="h-5 w-5 text-[var(--text-secondary)]" />
			</div>
		</div>
	</div>

	<!-- Trend Charts Section -->
	<div class="border border-[var(--surface-border)] bg-white p-6">
		<div class="flex items-center justify-between mb-6">
			<h2 class="text-lg font-semibold text-[var(--text-primary)]">
				Trend Charts
			</h2>
			<TimeWindowSelector
				selected={trendStats.currentWindow}
				onchange={(window) => trendStats.setWindow(window)}
			/>
		</div>

		{#if trendStats.error}
			<div class="p-4 bg-red-50 border border-red-200 text-red-700 rounded mb-6">
				{trendStats.error}
			</div>
		{/if}

		<div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
			{#await TrendChartPromise then TrendChart}
				<TrendChart.default
					title="Articles"
					dataPoints={trendStats.data?.data_points ?? []}
					dataKey="articles"
					color="#3b82f6"
					loading={trendStats.loading}
				/>
				<TrendChart.default
					title="Summarized"
					dataPoints={trendStats.data?.data_points ?? []}
					dataKey="summarized"
					color="#8b5cf6"
					loading={trendStats.loading}
				/>
				<TrendChart.default
					title="Feed Activity"
					dataPoints={trendStats.data?.data_points ?? []}
					dataKey="feed_activity"
					color="#10b981"
					loading={trendStats.loading}
				/>
			{/await}
		</div>
	</div>
{:else}
	<!-- Mobile -->
	<div
		class="h-screen overflow-hidden flex flex-col"
		style="background: var(--app-bg);"
	>
		<!-- Page Title -->
		<div class="px-5 pt-4 pb-2">
			<h1
				class="text-2xl font-bold text-center"
				style="color: var(--alt-primary); font-family: var(--font-outfit, sans-serif);"
			>
				Statistics
			</h1>
		</div>

		<div class="flex-1 min-h-0 flex flex-col px-5 py-4 overflow-y-auto">
			<!-- Connection Status Indicator -->
			<div class="flex justify-center mb-4">
				<div
					class="inline-flex items-center gap-2 px-3 py-1 rounded-full text-xs font-medium bg-[var(--bg-surface)] border border-[var(--border-glass)]"
				>
					<div
						class="w-2 h-2 rounded-full transition-colors {stats.isConnected
							? 'animate-pulse'
							: ''}"
						style="background-color: {stats.isConnected
							? 'var(--alt-success)'
							: 'var(--alt-warning)'}"
					></div>
					<span style="color: var(--text-secondary)">
						{stats.isConnected ? "Live Updates" : "Connecting..."}
					</span>
				</div>
			</div>

			{#if mobileLoading}
				<div class="flex flex-col items-center justify-center py-20">
					<div
						class="w-10 h-10 border-4 border-[var(--text-secondary)] border-t-[var(--accent-primary)] rounded-full animate-spin"
					></div>
					<p class="mt-4 text-[var(--text-secondary)] font-medium">
						Loading stats...
					</p>
				</div>
			{:else if mobileError}
				<div
					class="bg-[var(--bg-surface)] border border-red-500/20 rounded-2xl p-6 text-center"
				>
					<p class="text-red-400 font-medium mb-2">Error</p>
					<p class="text-[var(--text-secondary)] text-sm">{mobileError}</p>
				</div>
			{:else}
				<div class="grid grid-cols-1 gap-4">
					<div
						class="bg-[var(--bg-surface)] border border-[var(--border-glass)] rounded-2xl p-6 shadow-lg"
					>
						<div class="flex items-center gap-3 mb-3">
							<Rss class="w-5 h-5 text-[var(--alt-primary)]" />
							<span
								class="text-[var(--text-secondary)] text-sm uppercase tracking-wider font-semibold"
								>Total Feeds</span
							>
						</div>
						<span class="text-4xl font-bold text-[var(--text-primary)]">
							{formatNumber(displayFeedAmount)}
						</span>
					</div>

					<div
						class="bg-[var(--bg-surface)] border border-[var(--border-glass)] rounded-2xl p-6 shadow-lg"
					>
						<div class="flex items-center gap-3 mb-3">
							<FileText class="w-5 h-5 text-[var(--alt-primary)]" />
							<span
								class="text-[var(--text-secondary)] text-sm uppercase tracking-wider font-semibold"
								>Total Articles</span
							>
						</div>
						<span class="text-4xl font-bold text-[var(--text-primary)]">
							{formatNumber(displayTotalArticles)}
						</span>
					</div>

					<div
						class="bg-[var(--bg-surface)] border border-[var(--border-glass)] rounded-2xl p-6 shadow-lg"
					>
						<div class="flex items-center gap-3 mb-3">
							<Layers class="w-5 h-5 text-[var(--alt-primary)]" />
							<span
								class="text-[var(--text-secondary)] text-sm uppercase tracking-wider font-semibold"
								>Unsummarized</span
							>
						</div>
						<span class="text-4xl font-bold text-[var(--text-primary)]">
							{formatNumber(displayUnsummarized)}
						</span>
					</div>

					<div
						class="bg-[var(--bg-surface)] border border-[var(--border-glass)] rounded-2xl p-6 shadow-lg"
					>
						<div class="flex items-center gap-3 mb-3">
							<BookOpen class="w-5 h-5 text-[var(--alt-primary)]" />
							<span
								class="text-[var(--text-secondary)] text-sm uppercase tracking-wider font-semibold"
								>Today's Unread</span
							>
						</div>
						<span class="text-4xl font-bold text-[var(--text-primary)]">
							{formatNumber(unreadCount)}
						</span>
					</div>
				</div>
			{/if}
		</div>
	</div>
{/if}
