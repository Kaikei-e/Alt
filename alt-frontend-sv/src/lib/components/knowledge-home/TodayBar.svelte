<script lang="ts">
import {
	Newspaper,
	FileText,
	AlertCircle,
	CalendarRange,
	Activity,
	Search,
	BirdIcon,
} from "@lucide/svelte";
import { goto } from "$app/navigation";
import type { TodayDigestData } from "$lib/connect/knowledge_home";

interface Props {
	digest: TodayDigestData | null;
}

const { digest }: Props = $props();

let searchQuery = $state("");

function handleSearch() {
	if (searchQuery.trim()) {
		goto(`/feeds/search?q=${encodeURIComponent(searchQuery.trim())}`);
	}
}

function handleSearchKeydown(e: KeyboardEvent) {
	if (e.key === "Enter") {
		handleSearch();
	}
}
</script>

{#if digest}
	<div
		class="flex flex-wrap items-center gap-3 px-4 py-3 border-b border-[var(--surface-border)] bg-[var(--surface-bg)]"
	>
		<!-- Stat Chips -->
		<div class="flex items-center gap-3 flex-wrap">
			<span class="inline-flex items-center gap-1.5 text-xs text-[var(--text-secondary)]">
				<Newspaper class="h-3.5 w-3.5 text-blue-400" />
				<span class="font-medium text-[var(--text-primary)]">{digest.newArticles}</span> new
			</span>
			<span class="inline-flex items-center gap-1.5 text-xs text-[var(--text-secondary)]">
				<FileText class="h-3.5 w-3.5 text-teal-400" />
				<span class="font-medium text-[var(--text-primary)]">{digest.summarizedArticles}</span> summarized
			</span>
			{#if digest.unsummarizedArticles > 0}
				<span class="inline-flex items-center gap-1.5 text-xs text-[var(--text-secondary)]">
					<AlertCircle class="h-3.5 w-3.5 text-amber-400" />
					<span class="font-medium text-[var(--text-primary)]">{digest.unsummarizedArticles}</span> pending
				</span>
			{/if}
		</div>

		<!-- Top Tags -->
		{#if digest.topTags.length > 0}
			<div class="flex items-center gap-1">
				{#each digest.topTags.slice(0, 5) as tag}
					<span
						class="px-1.5 py-0.5 text-xs rounded bg-[var(--surface-hover)] text-[var(--text-secondary)]"
					>
						{tag}
					</span>
				{/each}
			</div>
		{/if}

		<!-- Spacer -->
		<div class="flex-1"></div>

		<!-- Shortcuts -->
		<div class="flex items-center gap-2">
			{#if digest.weeklyRecapAvailable}
				<a
					href="/recap"
					class="inline-flex items-center gap-1 px-2 py-1 text-xs rounded-md text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--surface-hover)] transition-colors"
				>
					<CalendarRange class="h-3.5 w-3.5" />
					Recap
				</a>
			{/if}
			{#if digest.eveningPulseAvailable}
				<a
					href="/recap/evening-pulse"
					class="inline-flex items-center gap-1 px-2 py-1 text-xs rounded-md text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--surface-hover)] transition-colors"
				>
					<Activity class="h-3.5 w-3.5" />
					Pulse
				</a>
			{/if}

			<!-- Search -->
			<div class="flex items-center gap-1">
				<div class="relative">
					<input
						type="text"
						bind:value={searchQuery}
						onkeydown={handleSearchKeydown}
						placeholder="Search..."
						class="w-32 px-2 py-1 pl-7 text-xs rounded-md bg-[var(--surface-hover)] text-[var(--text-primary)] placeholder:text-[var(--text-secondary)] border border-transparent focus:border-[var(--accent-primary)] focus:outline-none"
					/>
					<Search class="absolute left-2 top-1/2 -translate-y-1/2 h-3 w-3 text-[var(--text-secondary)]" />
				</div>
				<a
					href="/augur"
					class="p-1.5 rounded-md text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--surface-hover)] transition-colors"
					title="Ask Augur"
				>
					<BirdIcon class="h-3.5 w-3.5" />
				</a>
			</div>
		</div>
	</div>
{/if}
