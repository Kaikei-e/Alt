<script lang="ts">
	import { ArrowRight, Loader2 } from "@lucide/svelte";
	import { getFeedsWithCursorClient } from "$lib/api/client/feeds";
	import type { RenderFeed } from "$lib/schema/feed";
	import { onMount } from "svelte";

	const svBasePath = "/sv";

	// Simple state without TanStack Query
	let feeds = $state<RenderFeed[]>([]);
	let isLoading = $state(true);
	let error = $state<Error | null>(null);

	// Fetch latest 5 unread feeds on mount
	onMount(async () => {
		try {
			isLoading = true;
			const result = await getFeedsWithCursorClient(undefined, 5);
			feeds = result.data ?? [];
		} catch (err) {
			error = err as Error;
		} finally {
			isLoading = false;
		}
	});
</script>

<div class="border border-[var(--surface-border)] bg-white p-6 flex flex-col h-full overflow-x-hidden">
	<!-- Header -->
	<div class="flex items-center justify-between mb-4">
		<h3 class="text-lg font-semibold text-[var(--text-primary)]">Unread Feeds</h3>
		<a
			href="{svBasePath}/desktop/feeds"
			class="flex items-center gap-1 text-sm text-[var(--accent-primary)] hover:underline"
		>
			View All
			<ArrowRight class="h-3.5 w-3.5" />
		</a>
	</div>

	<!-- Content -->
	<div class="flex-1 overflow-y-auto overflow-x-hidden">
		{#if isLoading}
			<div class="flex items-center justify-center py-12">
				<Loader2 class="h-6 w-6 animate-spin text-[var(--accent-primary)]" />
			</div>
		{:else if error}
			<div class="text-sm text-[var(--alt-error)] text-center py-8">
				Error: {error.message}
			</div>
		{:else if feeds.length === 0}
			<div class="text-sm text-[var(--text-secondary)] text-center py-8">
				No unread feeds
			</div>
		{:else}
			<ul class="space-y-3">
				{#each feeds as feed}
					<li class="border-b border-[var(--surface-border)] pb-3 last:border-b-0 last:pb-0">
						<a
							href={feed.link}
							target="_blank"
							rel="noopener noreferrer"
							class="block hover:bg-[var(--surface-hover)] p-2 -m-2 transition-colors duration-200"
						>
							<h4 class="text-sm font-medium text-[var(--text-primary)] line-clamp-2 mb-1 break-words">
								{feed.title}
							</h4>
							{#if feed.excerpt}
								<p class="text-xs text-[var(--text-secondary)] line-clamp-2 break-words">
									{feed.excerpt}
								</p>
							{/if}
							{#if feed.publishedAtFormatted}
								<p class="text-xs text-[var(--text-muted)] mt-1">
									{feed.publishedAtFormatted}
								</p>
							{/if}
						</a>
					</li>
				{/each}
			</ul>
		{/if}
	</div>
</div>
