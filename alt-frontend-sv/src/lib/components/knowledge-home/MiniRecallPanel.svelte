<script lang="ts">
import { BirdIcon, Newspaper, Orbit, Shuffle } from "@lucide/svelte";
import type { TodayDigestData } from "$lib/connect/knowledge_home";

interface Props {
	digest: TodayDigestData | null;
}

const { digest }: Props = $props();
</script>

{#if digest}
	<aside
		class="border rounded-lg bg-[var(--surface-bg)] border-[var(--surface-border)] divide-y divide-[var(--surface-border)]"
	>
		<!-- Top Tags -->
		{#if digest.topTags.length > 0}
			<div class="p-4">
				<h3
					class="text-xs font-semibold text-[var(--text-secondary)] uppercase tracking-wider mb-2"
				>
					Today's Tags
				</h3>
				<div class="flex flex-wrap gap-1">
					{#each digest.topTags as tag}
						<span
							class="rounded border border-[var(--chip-border)] bg-[var(--chip-bg)] px-2 py-0.5 text-xs font-medium text-[var(--chip-text)]"
						>
							{tag}
						</span>
					{/each}
				</div>
			</div>
		{/if}

		<!-- Quick Links (navigation only, no digest-dependent conditionals) -->
		<div class="p-4">
			<h3
				class="text-xs font-semibold text-[var(--text-secondary)] uppercase tracking-wider mb-2"
			>
				Quick Links
			</h3>
			<div class="flex flex-col gap-0.5">
				<a
					href="/recap/morning-letter"
					class="flex items-center gap-2 rounded-md px-3 py-2 text-sm font-medium text-[var(--interactive-text)] hover:bg-[var(--action-surface)] hover:text-[var(--interactive-text-hover)] transition-colors"
				>
					<Newspaper class="h-4 w-4" />
					Morning Letter
				</a>
				<a
					href="/augur"
					class="flex items-center gap-2 rounded-md px-3 py-2 text-sm font-medium text-[var(--interactive-text)] hover:bg-[var(--action-surface)] hover:text-[var(--interactive-text-hover)] transition-colors"
				>
					<BirdIcon class="h-4 w-4" />
					Ask Augur
				</a>
			</div>
		</div>

		<!-- Explore -->
		<div class="p-4">
			<h3
				class="text-xs font-semibold text-[var(--text-secondary)] uppercase tracking-wider mb-2"
			>
				Explore
			</h3>
			<div class="flex flex-col gap-0.5">
				<a
					href="/feeds/tag-trail"
					class="flex items-center gap-2 rounded-md px-3 py-2 text-sm font-medium text-[var(--interactive-text)] hover:bg-[var(--action-surface)] hover:text-[var(--interactive-text-hover)] transition-colors"
				>
					<Shuffle class="h-4 w-4" />
					Tag Trail
				</a>
				<a
					href="/feeds/tag-verse"
					class="flex items-center gap-2 rounded-md px-3 py-2 text-sm font-medium text-[var(--interactive-text)] hover:bg-[var(--action-surface)] hover:text-[var(--interactive-text-hover)] transition-colors"
				>
					<Orbit class="h-4 w-4" />
					Tag Verse
				</a>
			</div>
		</div>
	</aside>
{/if}
