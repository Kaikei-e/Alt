<script lang="ts">
import {
	Activity,
	AlertCircle,
	CalendarRange,
	Clock,
	FileText,
	Newspaper,
	Sparkles,
} from "@lucide/svelte";
import type {
	ServiceQuality,
	TodayDigestData,
} from "$lib/connect/knowledge_home";

interface Props {
	digest: TodayDigestData | null;
	serviceQuality?: ServiceQuality;
}

const { digest, serviceQuality = "full" }: Props = $props();

const isFallback = $derived(
	serviceQuality === "fallback" && digest?.digestFreshness === "unknown",
);
</script>

{#if !digest}
	<div
		class="flex items-center gap-2 px-4 py-3 border-b border-[var(--surface-border)] bg-[var(--surface-bg)] shadow-[var(--shadow-sm)]"
	>
		<AlertCircle class="h-4 w-4 text-[var(--text-secondary)]" />
		<span class="text-sm text-[var(--text-secondary)]"
			>Today's digest is still being prepared.</span
		>
	</div>
{:else if isFallback}
	<div
		class="flex items-center gap-2 px-4 py-3 border-b border-[var(--surface-border)] bg-[var(--surface-bg)] shadow-[var(--shadow-sm)]"
	>
		<AlertCircle class="h-4 w-4 text-[var(--badge-amber-text)]" />
		<span class="text-sm text-[var(--text-secondary)]"
			>Digest section is temporarily unavailable or stale.</span
		>
	</div>
{:else}
	<div
		class="flex flex-col border-b border-[var(--surface-border)] bg-[var(--surface-bg)] shadow-[var(--shadow-sm)]"
	>
		<!-- Row 1: Action Shortcuts -->
		<div
			class="flex items-center gap-2 px-4 py-2 border-b border-[var(--surface-border)]/30"
		>
			<a
				href="/recap/morning-letter"
				class="inline-flex items-center gap-1.5 rounded-lg border border-[var(--chip-border)] bg-[var(--action-surface)] px-3 py-2 text-sm font-medium text-[var(--interactive-text)] hover:bg-[var(--action-surface-hover)] hover:text-[var(--interactive-text-hover)] transition-colors"
			>
				<Newspaper class="h-4 w-4" />
				Morning Letter
			</a>

			{#if digest.eveningPulseAvailable}
				<a
					href="/recap/evening-pulse"
					class="inline-flex items-center gap-1.5 rounded-lg border border-[var(--chip-border)] bg-[var(--action-surface)] px-3 py-2 text-sm font-medium text-[var(--interactive-text)] hover:bg-[var(--action-surface-hover)] hover:text-[var(--interactive-text-hover)] transition-colors"
				>
					<Activity class="h-4 w-4" />
					Pulse
					{#if digest.needToKnowCount > 0}
						<span
							class="inline-flex items-center justify-center min-w-[1.25rem] h-5 px-1 text-xs font-semibold rounded-full bg-[var(--badge-orange-bg)] text-[var(--badge-orange-text)]"
							title="{digest.needToKnowCount} need-to-know"
						>
							{digest.needToKnowCount}
						</span>
					{/if}
				</a>
			{:else}
				<span
					class="inline-flex cursor-default items-center gap-1.5 rounded-lg border border-[var(--surface-border)] px-3 py-2 text-sm text-[var(--text-secondary)]/55"
					title="No pulse content available today"
				>
					<Activity class="h-4 w-4" />
					Pulse
				</span>
			{/if}

			{#if digest.weeklyRecapAvailable}
				<a
					href="/recap"
					class="inline-flex items-center gap-1.5 rounded-lg border border-[var(--chip-border)] bg-[var(--action-surface)] px-3 py-2 text-sm font-medium text-[var(--interactive-text)] hover:bg-[var(--action-surface-hover)] hover:text-[var(--interactive-text-hover)] transition-colors"
				>
					<CalendarRange class="h-4 w-4" />
					Recap
				</a>
			{:else}
				<span
					class="inline-flex cursor-default items-center gap-1.5 rounded-lg border border-[var(--surface-border)] px-3 py-2 text-sm text-[var(--text-secondary)]/55"
					title="No recap available today"
				>
					<CalendarRange class="h-4 w-4" />
					Recap
				</span>
			{/if}

			{#if digest.digestFreshness === "stale"}
				<span
					class="inline-flex items-center gap-1 ml-auto text-xs text-[var(--badge-amber-text)]"
					title="Data may be outdated"
				>
					<Clock class="h-3 w-3" />
					Stale
				</span>
			{/if}
		</div>

		<!-- Row 2: Theme + Tags -->
		<div class="flex flex-wrap items-center gap-3 px-4 py-2">
			<!-- Stat Chips -->
			<div class="flex items-center gap-3 flex-wrap">
				<span
					class="inline-flex items-center gap-1.5 text-xs text-[var(--text-secondary)]"
				>
					<Sparkles class="h-3.5 w-3.5 text-[var(--badge-blue-text)]" />
					<span class="text-base font-bold text-[var(--text-primary)]"
						>{digest.newArticles}</span
					> new
				</span>
				<span
					class="inline-flex items-center gap-1.5 text-xs text-[var(--text-secondary)]"
				>
					<FileText class="h-3.5 w-3.5 text-[var(--badge-teal-text)]" />
					<span class="text-base font-bold text-[var(--text-primary)]"
						>{digest.summarizedArticles}</span
					> summarized
				</span>
				{#if digest.unsummarizedArticles > 0}
					<span
						class="inline-flex items-center gap-1.5 text-xs text-[var(--text-secondary)]"
					>
						<AlertCircle class="h-3.5 w-3.5 text-[var(--badge-amber-text)]" />
						<span class="text-base font-bold text-[var(--text-primary)]"
							>{digest.unsummarizedArticles}</span
						> pending
					</span>
				{/if}
			</div>

			{#if digest.primaryTheme || digest.topTags.length > 0}
				<p class="text-xs text-[var(--text-secondary)]">
					{digest.primaryTheme ?? digest.topTags.slice(0, 3).join(", ")}
				</p>
			{/if}

			<!-- Top Tags -->
			{#if digest.topTags.length > 0}
				<div class="flex items-center gap-1 ml-auto">
					{#each digest.topTags.slice(0, 5) as tag}
						<span
							class="rounded border border-[var(--chip-border)] bg-[var(--chip-bg)] px-2 py-0.5 text-xs font-medium text-[var(--chip-text)]"
						>
							{tag}
						</span>
					{/each}
				</div>
			{/if}
		</div>
	</div>
{/if}
