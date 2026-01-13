<script lang="ts">
	import { BarChart3, TrendingUp, FileText, CheckCircle } from "@lucide/svelte";
	import { onMount } from "svelte";
	import PageHeader from "$lib/components/desktop/layout/PageHeader.svelte";
	import { useFeedStats } from "$lib/hooks/useFeedStats.svelte";
	import { useTrendStats } from "$lib/hooks/useTrendStats.svelte";
	import TrendChart from "$lib/components/desktop/stats/TrendChart.svelte";
	import TimeWindowSelector from "$lib/components/desktop/stats/TimeWindowSelector.svelte";

	const stats = useFeedStats();
	const trendStats = useTrendStats();

	let feedAmount = $derived(stats.feedAmount);
	let totalArticlesAmount = $derived(stats.totalArticlesAmount);
	let unsummarizedArticlesAmount = $derived(stats.unsummarizedArticlesAmount);
	let summarizedArticles = $derived(totalArticlesAmount - unsummarizedArticlesAmount);
	let connectionStatus = $derived(stats.isConnected ? "Connected" : "Disconnected");

	let statCards = $derived([
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

	// Fetch trend data on mount
	onMount(() => {
		trendStats.fetchData("24h");
	});
</script>

<PageHeader title="Statistics" description="Overview of your RSS feed analytics" />

<!-- Stats cards -->
<div class="grid grid-cols-1 md:grid-cols-3 gap-6 mb-8">
	{#each statCards as stat}
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
		<TrendingUp class="h-5 w-5 text-[var(--text-secondary)]" />
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
		<TrendChart
			title="Articles"
			dataPoints={trendStats.data?.data_points ?? []}
			dataKey="articles"
			color="#3b82f6"
			loading={trendStats.loading}
		/>
		<TrendChart
			title="Summarized"
			dataPoints={trendStats.data?.data_points ?? []}
			dataKey="summarized"
			color="#8b5cf6"
			loading={trendStats.loading}
		/>
		<TrendChart
			title="Feed Activity"
			dataPoints={trendStats.data?.data_points ?? []}
			dataKey="feed_activity"
			color="#10b981"
			loading={trendStats.loading}
		/>
	</div>
</div>
