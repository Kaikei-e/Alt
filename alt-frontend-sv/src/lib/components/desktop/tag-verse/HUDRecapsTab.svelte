<script lang="ts">
import {
	createClientTransport,
	type RecapSearchResultItem,
	searchRecapsByTag,
} from "#lib/connect/index.js";
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

const groupedResults = $derived<DateGroup[]>(groupByDate(results));

function groupByDate(items: RecapSearchResultItem[]): DateGroup[] {
	const groups = new Map<string, RecapSearchResultItem[]>();
	for (const item of items) {
		const date = formatDateKey(item.executedAt);
		if (!groups.has(date)) {
			groups.set(date, []);
		}
		groups.get(date)?.push(item);
	}
	return Array.from(groups.entries()).map(([date, groupItems]) => ({
		date,
		items: groupItems,
	}));
}

function formatDateKey(dateStr: string): string {
	try {
		return new Date(dateStr).toLocaleDateString("en-CA", {
			year: "numeric",
			month: "2-digit",
			day: "2-digit",
		});
	} catch {
		return dateStr;
	}
}

let lastTag: string | null = null;
let requestSeq = 0;

$effect(() => {
	const currentTag = tagName;
	if (currentTag === lastTag) {
		return;
	}
	lastTag = currentTag;
	const seq = ++requestSeq;
	results = [];
	isLoading = true;
	error = null;

	const transport = createClientTransport();
	searchRecapsByTag(transport, currentTag, 50)
		.then((items) => {
			if (seq !== requestSeq) return;
			results = items;
		})
		.catch((e) => {
			if (seq !== requestSeq) return;
			error = e instanceof Error ? e.message : "Failed to search recaps";
		})
		.finally(() => {
			if (seq === requestSeq) {
				isLoading = false;
			}
		});
});
</script>

<div class="flex flex-1 flex-col overflow-y-auto">
	{#if error}
		<div class="px-4 py-4 text-center text-sm text-[var(--alt-error)]">
			{error}
		</div>
	{/if}

	{#if isLoading}
		<div
			class="flex items-center justify-center py-8 text-xs text-[var(--text-muted)]"
			style="font-family: var(--font-mono);"
		>
			Loading recaps...
		</div>
	{:else if groupedResults.length === 0 && !error}
		<div class="px-4 py-8 text-center text-sm text-[var(--text-muted)]">
			<p>No related recaps found for "{tagName}"</p>
			<p class="mt-1 text-xs">
				This tag may not appear in any recap's top terms
			</p>
		</div>
	{:else}
		{#each groupedResults as group (group.date)}
			<div
				class="sticky top-0 z-10 border-b border-[var(--surface-border)] bg-[var(--surface-2)] px-4 py-1.5"
			>
				<span
					class="text-xs text-[var(--text-muted)]"
					style="font-family: var(--font-mono);"
				>
					{group.date}
				</span>
			</div>

			<div class="divide-y divide-[var(--surface-border)] border-b border-[var(--surface-border)]">
				{#each group.items as item (item.jobId + item.genre)}
					<button
						type="button"
						class="w-full cursor-pointer px-4 py-3 text-left transition-colors hover:bg-[var(--surface-hover)]"
						onclick={() => {
							selectedRecap = item;
							modalOpen = true;
						}}
					>
						<div class="mb-1.5 flex items-baseline justify-between gap-2">
							<h4
								class="text-sm font-semibold text-[var(--text-primary)]"
								style="font-family: var(--font-body);"
							>
								{item.genre}
							</h4>
							<span
								class="text-xs text-[var(--text-muted)]"
								style="font-family: var(--font-mono);"
							>
								{group.date}
							</span>
						</div>

						{#if item.bullets.length > 0}
							<ul class="mb-2 space-y-1">
								{#each item.bullets as bullet}
									<li
										class="relative pl-3 text-xs text-[var(--text-secondary)] before:absolute before:left-0 before:text-[var(--text-muted)] before:content-['•']"
									>
										{bullet}
									</li>
								{/each}
							</ul>
						{:else if item.summary}
							<p
								class="mb-2 line-clamp-3 text-sm leading-relaxed text-[var(--text-secondary)]"
							>
								{item.summary}
							</p>
						{/if}

						{#if item.topTerms.length > 0}
							<div class="mt-2 flex flex-wrap gap-1">
								{#each item.topTerms.slice(0, 6) as term}
									<span
										class="rounded-none border border-[var(--chip-border)] bg-[var(--chip-bg)] px-1.5 py-0.5 text-[10px] text-[var(--chip-text)]"
										style="font-family: var(--font-mono);"
									>
										{term}
									</span>
								{/each}
							</div>
						{/if}
					</button>
				{/each}
			</div>
		{/each}
	{/if}
</div>

<RecapDetailModal
	recap={selectedRecap}
	open={modalOpen}
	onOpenChange={(v) => {
		modalOpen = v;
	}}
/>
