<script lang="ts">
	import { Rss, FileText, CheckCircle, Wifi, WifiOff } from "@lucide/svelte";
	import { useSSEFeedsStats } from "$lib/hooks/useSSEFeedsStats.svelte";

	// Use SSE hook for real-time stats
	const stats = useSSEFeedsStats();

	// Use $derived to maintain reactivity - destructuring loses reactivity in Svelte 5
	let feedAmount = $derived(stats.feedAmount);
	let totalArticlesAmount = $derived(stats.totalArticlesAmount);
	let unsummarizedArticlesAmount = $derived(stats.unsummarizedArticlesAmount);
	let isConnected = $derived(stats.isConnected);
	let summarizedArticles = $derived(totalArticlesAmount - unsummarizedArticlesAmount);
</script>

<div class="border border-[var(--surface-border)] bg-white p-6">
	<div class="grid grid-cols-4 gap-6">
		<!-- Feed Count -->
		<div class="flex items-center gap-3">
			<div class="flex-shrink-0">
				<div
					class="w-10 h-10 flex items-center justify-center bg-[var(--alt-blue)]/10 text-[var(--alt-blue)]"
				>
					<Rss class="h-5 w-5" />
				</div>
			</div>
			<div class="flex-1 min-w-0">
				<p class="text-xs text-[var(--text-secondary)]">Feed Count</p>
				<p class="text-xl font-semibold text-[var(--text-primary)] tabular-nums">
					{feedAmount.toLocaleString()}
				</p>
			</div>
		</div>

		<!-- Total Articles -->
		<div class="flex items-center gap-3">
			<div class="flex-shrink-0">
				<div
					class="w-10 h-10 flex items-center justify-center bg-[var(--alt-purple)]/10 text-[var(--alt-purple)]"
				>
					<FileText class="h-5 w-5" />
				</div>
			</div>
			<div class="flex-1 min-w-0">
				<p class="text-xs text-[var(--text-secondary)]">Total Articles</p>
				<p class="text-xl font-semibold text-[var(--text-primary)] tabular-nums">
					{totalArticlesAmount.toLocaleString()}
				</p>
			</div>
		</div>

		<!-- Summarized Articles -->
		<div class="flex items-center gap-3">
			<div class="flex-shrink-0">
				<div
					class="w-10 h-10 flex items-center justify-center bg-[var(--alt-success)]/10 text-[var(--alt-success)]"
				>
					<CheckCircle class="h-5 w-5" />
				</div>
			</div>
			<div class="flex-1 min-w-0">
				<p class="text-xs text-[var(--text-secondary)]">Summarized</p>
				<p class="text-xl font-semibold text-[var(--text-primary)] tabular-nums">
					{summarizedArticles.toLocaleString()}
				</p>
			</div>
		</div>

		<!-- Connection Status -->
		<div class="flex items-center gap-3">
			<div class="flex-shrink-0">
				<div
					class="w-10 h-10 flex items-center justify-center {isConnected
						? 'bg-[var(--alt-success)]/10 text-[var(--alt-success)]'
						: 'bg-[var(--alt-error)]/10 text-[var(--alt-error)]'}"
				>
					{#if isConnected}
						<Wifi class="h-5 w-5" />
					{:else}
						<WifiOff class="h-5 w-5" />
					{/if}
				</div>
			</div>
			<div class="flex-1 min-w-0">
				<p class="text-xs text-[var(--text-secondary)]">Connection</p>
				<p
					class="text-sm font-medium {isConnected
						? 'text-[var(--alt-success)]'
						: 'text-[var(--alt-error)]'}"
				>
					{isConnected ? 'Connected' : 'Disconnected'}
				</p>
			</div>
		</div>
	</div>
</div>
