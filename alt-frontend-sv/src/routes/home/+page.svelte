<script lang="ts">
import { BookOpen, FileText, Layers, Rss } from "@lucide/svelte";
import FloatingMenu from "$lib/components/mobile/feeds/swipe/FloatingMenu.svelte";
import { useSSEFeedsStats } from "$lib/hooks/useSSEFeedsStats.svelte";

interface StatsData {
	feed_amount: { amount: number };
	total_articles: { amount: number };
	unsummarized_articles: { amount: number };
}

interface PageData {
	stats: StatsData;
	unreadCount: number;
	error?: string;
}

interface Props {
	data: PageData;
}

const { data }: Props = $props();

// SSE接続を確立
const sseStats = useSSEFeedsStats();

// 初期値としてサーバーサイドデータを使用、その後SSEで更新
let feedAmount = $state(0);
let totalArticlesAmount = $state(0);
let unsummarizedArticlesAmount = $state(0);

// SSEデータとサーバーデータを監視して更新
$effect(() => {
	// SSEデータが利用可能な場合はそれを使用、そうでない場合はサーバーデータを使用
	feedAmount =
		sseStats.feedAmount > 0
			? sseStats.feedAmount
			: data.stats.feed_amount.amount;
	totalArticlesAmount =
		sseStats.totalArticlesAmount > 0
			? sseStats.totalArticlesAmount
			: data.stats.total_articles.amount;
	unsummarizedArticlesAmount =
		sseStats.unsummarizedArticlesAmount > 0
			? sseStats.unsummarizedArticlesAmount
			: data.stats.unsummarized_articles.amount;
});
</script>

<div class="p-8 max-w-4xl mx-auto" data-style="alt-paper">
	<h1 class="text-3xl font-bold mb-6" style="color: var(--text-primary);">
		Statistics
	</h1>

	<!-- 接続状態インジケータ -->
	<div
		class="p-4 border rounded-lg shadow-sm mb-6"
		style="
			background: var(--surface-bg);
			border-color: var(--surface-border);
			box-shadow: var(--shadow-sm);
		"
	>
		<div class="flex items-center gap-2">
			<div
				class="w-2 h-2 rounded-full transition-colors"
				style="
					background-color: {sseStats.isConnected
						? 'var(--alt-success)'
						: sseStats.retryCount > 0
							? 'var(--alt-warning)'
							: 'var(--alt-error)'};
				"
			></div>
			<p class="text-sm" style="color: var(--text-primary);">
				{sseStats.isConnected
					? "Connected"
					: sseStats.retryCount > 0
						? `Reconnecting (${sseStats.retryCount}/3)`
						: "Disconnected"}
			</p>
		</div>
	</div>

	{#if data.error}
		<div
			class="p-4 bg-yellow-50 dark:bg-yellow-900/20 text-yellow-700 dark:text-yellow-300 rounded-md mb-6"
		>
			<p class="font-medium">⚠️ {data.error}</p>
		</div>
	{/if}

	<div class="grid grid-cols-1 md:grid-cols-2 gap-6">
		<!-- 全フィードの件数 -->
		<div
			class="p-6 border rounded-lg shadow-sm"
			style="
				background: var(--surface-bg);
				border-color: var(--surface-border);
				box-shadow: var(--shadow-sm);
			"
		>
			<div class="flex items-center gap-3 mb-4">
				<Rss size={24} style="color: var(--alt-primary);" />
				<h3
					class="text-sm font-semibold uppercase tracking-wider"
					style="color: var(--text-primary);"
				>
					Total Feeds
				</h3>
			</div>
			<p
				class="text-3xl font-bold mb-2"
				style="color: var(--text-primary);"
			>
				{feedAmount.toLocaleString()}
			</p>
			<p class="text-sm" style="color: var(--text-muted);">
				RSS feeds being monitored
			</p>
		</div>

		<!-- 保存されている全Article数 -->
		<div
			class="p-6 border rounded-lg shadow-sm"
			style="
				background: var(--surface-bg);
				border-color: var(--surface-border);
				box-shadow: var(--shadow-sm);
			"
		>
			<div class="flex items-center gap-3 mb-4">
				<FileText size={24} style="color: var(--alt-primary);" />
				<h3
					class="text-sm font-semibold uppercase tracking-wider"
					style="color: var(--text-primary);"
				>
					Total Articles
				</h3>
			</div>
			<p
				class="text-3xl font-bold mb-2"
				style="color: var(--text-primary);"
			>
				{totalArticlesAmount.toLocaleString()}
			</p>
			<p class="text-sm" style="color: var(--text-muted);">
				All articles across RSS feeds
			</p>
		</div>

		<!-- 未要約のArticle数 -->
		<div
			class="p-6 border rounded-lg shadow-sm"
			style="
				background: var(--surface-bg);
				border-color: var(--surface-border);
				box-shadow: var(--shadow-sm);
			"
		>
			<div class="flex items-center gap-3 mb-4">
				<Layers size={24} style="color: var(--alt-primary);" />
				<h3
					class="text-sm font-semibold uppercase tracking-wider"
					style="color: var(--text-primary);"
				>
					Unsummarized Articles
				</h3>
			</div>
			<p
				class="text-3xl font-bold mb-2"
				style="color: var(--text-primary);"
			>
				{unsummarizedArticlesAmount.toLocaleString()}
			</p>
			<p class="text-sm" style="color: var(--text-muted);">
				Articles waiting for AI summarization
			</p>
		</div>

		<!-- 今日の未読フィード数 -->
		<div
			class="p-6 border rounded-lg shadow-sm"
			style="
				background: var(--surface-bg);
				border-color: var(--surface-border);
				box-shadow: var(--shadow-sm);
			"
		>
			<div class="flex items-center gap-3 mb-4">
				<BookOpen size={24} style="color: var(--alt-primary);" />
				<h3
					class="text-sm font-semibold uppercase tracking-wider"
					style="color: var(--text-primary);"
				>
					Today's Unread
				</h3>
			</div>
			<p
				class="text-3xl font-bold mb-2"
				style="color: var(--text-primary);"
			>
				{data.unreadCount.toLocaleString()}
			</p>
			<p class="text-sm" style="color: var(--text-muted);">
				Unread feeds from today
			</p>
		</div>
	</div>
	<FloatingMenu />
</div>

