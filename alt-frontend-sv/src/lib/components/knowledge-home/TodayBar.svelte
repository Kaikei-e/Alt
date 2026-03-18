<script lang="ts">
import {
	Newspaper,
	FileText,
	AlertCircle,
	CalendarRange,
	Activity,
	Sparkles,
} from "@lucide/svelte";
import type { TodayDigestData } from "$lib/connect/knowledge_home";

interface Props {
	digest: TodayDigestData | null;
}

const { digest }: Props = $props();
</script>

{#if digest}
	<div
		class="flex flex-col border-b border-[var(--surface-border)] bg-[var(--surface-bg)]"
	>
		<!-- Row 1: Action Shortcuts -->
		<div class="flex items-center gap-2 px-4 py-2 border-b border-[var(--surface-border)]/50">
			<a
				href="/recap/morning-letter"
				class="inline-flex items-center gap-1.5 px-2.5 py-1.5 text-xs font-medium rounded-md text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--surface-hover)] transition-colors"
			>
				<Newspaper class="h-3.5 w-3.5" />
				Morning Letter
			</a>

			{#if digest.eveningPulseAvailable}
				<a
					href="/recap/evening-pulse"
					class="inline-flex items-center gap-1.5 px-2.5 py-1.5 text-xs font-medium rounded-md text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--surface-hover)] transition-colors"
				>
					<Activity class="h-3.5 w-3.5" />
					Pulse
					{#if digest.needToKnowCount > 0}
						<span
							class="inline-flex items-center justify-center min-w-[1.25rem] h-5 px-1 text-xs font-semibold rounded-full bg-orange-500/15 text-orange-400"
							title="{digest.needToKnowCount} need-to-know"
						>
							{digest.needToKnowCount}
						</span>
					{/if}
				</a>
			{:else}
				<span
					class="inline-flex items-center gap-1.5 px-2.5 py-1.5 text-xs rounded-md text-[var(--text-secondary)]/50 cursor-default"
				>
					<Activity class="h-3.5 w-3.5" />
					Pulse
				</span>
			{/if}

			{#if digest.weeklyRecapAvailable}
				<a
					href="/recap"
					class="inline-flex items-center gap-1.5 px-2.5 py-1.5 text-xs font-medium rounded-md text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--surface-hover)] transition-colors"
				>
					<CalendarRange class="h-3.5 w-3.5" />
					Recap
				</a>
			{:else}
				<span
					class="inline-flex items-center gap-1.5 px-2.5 py-1.5 text-xs rounded-md text-[var(--text-secondary)]/50 cursor-default"
				>
					<CalendarRange class="h-3.5 w-3.5" />
					Recap
				</span>
			{/if}
		</div>

		<!-- Row 2: Stats + Tags -->
		<div class="flex flex-wrap items-center gap-3 px-4 py-2">
			<!-- Stat Chips -->
			<div class="flex items-center gap-3 flex-wrap">
				<span class="inline-flex items-center gap-1.5 text-xs text-[var(--text-secondary)]">
					<Sparkles class="h-3.5 w-3.5 text-blue-400" />
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
				<div class="flex items-center gap-1 ml-auto">
					{#each digest.topTags.slice(0, 5) as tag}
						<span
							class="px-1.5 py-0.5 text-xs rounded bg-[var(--surface-hover)] text-[var(--text-secondary)]"
						>
							{tag}
						</span>
					{/each}
				</div>
			{/if}
		</div>
	</div>
{/if}
