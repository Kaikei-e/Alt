<script lang="ts">
import { BookOpen, FileText, Layers, Rss } from "@lucide/svelte";

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
</script>

<div class="p-8 max-w-4xl mx-auto" data-style="alt-paper">
	<h1 class="text-3xl font-bold mb-6" style="color: var(--text-primary);">
		Statistics
	</h1>

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
				{data.stats.feed_amount.amount.toLocaleString()}
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
				{data.stats.total_articles.amount.toLocaleString()}
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
				{data.stats.unsummarized_articles.amount.toLocaleString()}
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
</div>

