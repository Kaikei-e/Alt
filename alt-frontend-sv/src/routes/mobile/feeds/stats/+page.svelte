<script lang="ts">
import { BookOpen, FileText, Layers, Rss } from "@lucide/svelte";
import { onMount } from "svelte";
import {
	getDetailedFeedStatsClient,
	getUnreadCountClient,
} from "$lib/api/client/feeds";
import FloatingMenu from "$lib/components/mobile/feeds/swipe/FloatingMenu.svelte";
import { useFeedStats } from "$lib/hooks/useFeedStats.svelte";
import type { DetailedFeedStatsSummary } from "$lib/schema/stats";

let stats: DetailedFeedStatsSummary | null = $state(null);
let unreadCount = $state(0);
let loading = $state(true);
let error: string | null = $state(null);

// Unified feed stats hook (SSE or Connect-RPC Streaming)
const sseStats = useFeedStats();

// Reactive state for display values
let displayFeedAmount = $state(0);
let displayTotalArticles = $state(0);
let displayUnsummarized = $state(0);

onMount(async () => {
	try {
		const [statsData, unreadData] = await Promise.all([
			getDetailedFeedStatsClient(),
			getUnreadCountClient(),
		]);
		stats = statsData;

		// Initialize display values with fetched data
		displayFeedAmount = statsData.feed_amount.amount;
		displayTotalArticles = statsData.total_articles.amount;
		displayUnsummarized = statsData.unsummarized_articles.amount;

		// Handle unread count response structure which wraps count in an object
		unreadCount = unreadData.count;
	} catch (e) {
		console.error("Failed to fetch stats", e);
		error = "Failed to load statistics";
	} finally {
		loading = false;
	}
});

// Synchronize SSE updates with display values
$effect(() => {
	if (sseStats.isConnected) {
		if (sseStats.feedAmount > 0) {
			displayFeedAmount = sseStats.feedAmount;
		}
		if (sseStats.totalArticlesAmount > 0) {
			displayTotalArticles = sseStats.totalArticlesAmount;
		}
		if (sseStats.unsummarizedArticlesAmount > 0) {
			displayUnsummarized = sseStats.unsummarizedArticlesAmount;
		}
	}
});

// Helper to format numbers nicely
function formatNumber(num: number): string {
	return new Intl.NumberFormat().format(num);
}
</script>

<svelte:head>
	<title>Statistics - Alt</title>
</svelte:head>

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
    <!-- Connection Status Indicator (Subtle) -->
    <div class="flex justify-center mb-4">
      <div
        class="inline-flex items-center gap-2 px-3 py-1 rounded-full text-xs font-medium bg-[var(--bg-surface)] border border-[var(--border-glass)]"
      >
        <div
          class="w-2 h-2 rounded-full transition-colors {sseStats.isConnected
            ? 'animate-pulse'
            : ''}"
          style="background-color: {sseStats.isConnected
            ? 'var(--alt-success)'
            : 'var(--alt-warning)'}"
        ></div>
        <span style="color: var(--text-secondary)">
          {sseStats.isConnected ? "Live Updates" : "Connecting..."}
        </span>
      </div>
    </div>

    {#if loading}
      <div class="flex flex-col items-center justify-center py-20">
        <div
          class="w-10 h-10 border-4 border-[var(--text-secondary)] border-t-[var(--accent-primary)] rounded-full animate-spin"
        ></div>
        <p class="mt-4 text-[var(--text-secondary)] font-medium">
          Loading stats...
        </p>
      </div>
    {:else if error}
      <div
        class="bg-[var(--bg-surface)] border border-red-500/20 rounded-2xl p-6 text-center"
      >
        <p class="text-red-400 font-medium mb-2">Error</p>
        <p class="text-[var(--text-secondary)] text-sm">{error}</p>
      </div>
    {:else}
      <div class="grid grid-cols-1 gap-4">
        <!-- Total Feeds Card -->
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
          <span
            class="text-4xl font-bold bg-clip-text text-[var(--text-primary)]"
          >
            {formatNumber(displayFeedAmount)}
          </span>
        </div>

        <!-- Total Articles Card -->
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

        <!-- Unsummarized Articles Card -->
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

        <!-- Today's Unread Card -->
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

  <FloatingMenu />
</div>
