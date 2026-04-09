<script lang="ts">
import { onMount } from "svelte";
import {
	createClientTransport,
	searchRecapsByTag,
	type RecapSearchResultItem,
} from "$lib/connect";
import { Loader2 } from "@lucide/svelte";
import RecapDetailModal from "./RecapDetailModal.svelte";

interface Props {
	tagName: string;
}

let { tagName }: Props = $props();

let results = $state<RecapSearchResultItem[]>([]);
let isLoading = $state(true);
let error = $state<string | null>(null);
let selectedRecap = $state<RecapSearchResultItem | null>(null);
let modalOpen = $state(false);

interface DateGroup {
	date: string;
	items: RecapSearchResultItem[];
}

let groupedResults = $derived<DateGroup[]>(groupByDate(results));

function groupByDate(items: RecapSearchResultItem[]): DateGroup[] {
	const groups = new Map<string, RecapSearchResultItem[]>();
	for (const item of items) {
		const date = formatDateKey(item.executedAt);
		if (!groups.has(date)) {
			groups.set(date, []);
		}
		groups.get(date)?.push(item);
	}
	return Array.from(groups.entries()).map(([date, items]) => ({ date, items }));
}

function formatDateKey(dateStr: string): string {
	try {
		return new Date(dateStr).toLocaleDateString("ja-JP", {
			year: "numeric",
			month: "2-digit",
			day: "2-digit",
		});
	} catch {
		return dateStr;
	}
}

async function loadRecaps() {
	isLoading = true;
	error = null;
	try {
		const transport = createClientTransport();
		results = await searchRecapsByTag(transport, tagName, 50);
	} catch (e) {
		error = e instanceof Error ? e.message : "Failed to search recaps";
	} finally {
		isLoading = false;
	}
}

onMount(() => {
	loadRecaps();
});

// Reload when tag changes
$effect(() => {
	tagName; // track dependency
	results = [];
	loadRecaps();
});
</script>

<div class="flex flex-col gap-3 overflow-y-auto flex-1 pr-1">
	{#if error}
		<div class="text-red-400 text-sm py-4 text-center">{error}</div>
	{/if}

	{#if isLoading}
		<div class="flex items-center justify-center py-8">
			<Loader2 class="h-5 w-5 animate-spin text-cyan-400" />
		</div>
	{:else if groupedResults.length === 0 && !error}
		<div class="text-white/40 text-sm py-8 text-center">
			<p>No related recaps found for "{tagName}"</p>
			<p class="mt-1 text-xs">This tag may not appear in any recap's top terms</p>
		</div>
	{:else}
		{#each groupedResults as group (group.date)}
			<!-- Date header -->
			<div class="sticky top-0 z-10 bg-[rgba(10,10,30,0.95)] py-1.5">
				<span class="text-xs font-semibold text-cyan-400/80 tracking-wide">{group.date}</span>
			</div>

			{#each group.items as item (item.jobId + item.genre)}
				<button
					type="button"
					class="rounded-lg border border-white/10 bg-white/5 p-4 text-left w-full cursor-pointer transition-colors hover:bg-white/10 hover:border-white/20"
					onclick={() => { selectedRecap = item; modalOpen = true; }}
				>
					<h4 class="text-sm font-semibold text-cyan-300 mb-2">
						{item.genre}
					</h4>

					{#if item.bullets.length > 0}
						<ul class="space-y-1 mb-2">
							{#each item.bullets as bullet}
								<li class="text-xs text-white/60 pl-3 relative before:content-['•'] before:absolute before:left-0 before:text-cyan-500/60">
									{bullet}
								</li>
							{/each}
						</ul>
					{:else if item.summary}
						<p class="text-sm text-white/70 mb-2 leading-relaxed line-clamp-3">
							{item.summary}
						</p>
					{/if}

					{#if item.topTerms.length > 0}
						<div class="flex flex-wrap gap-1 mt-2">
							{#each item.topTerms.slice(0, 6) as term}
								<span class="rounded-full bg-white/10 px-2 py-0.5 text-[10px] text-white/50">
									{term}
								</span>
							{/each}
						</div>
					{/if}
				</button>
			{/each}
		{/each}
	{/if}
</div>

<RecapDetailModal recap={selectedRecap} open={modalOpen} onOpenChange={(v) => { modalOpen = v; }} />
