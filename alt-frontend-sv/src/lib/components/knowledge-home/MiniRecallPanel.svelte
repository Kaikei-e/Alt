<script lang="ts">
import {
	CalendarRange,
	Activity,
	Newspaper,
	BirdIcon,
	Shuffle,
	Orbit,
} from "@lucide/svelte";
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
							class="px-2 py-0.5 text-xs rounded bg-[var(--surface-hover)] text-[var(--text-secondary)]"
						>
							{tag}
						</span>
					{/each}
				</div>
			</div>
		{/if}

		<!-- Quick Links -->
		<div class="p-4">
			<h3
				class="text-xs font-semibold text-[var(--text-secondary)] uppercase tracking-wider mb-2"
			>
				Quick Links
			</h3>
			<div class="flex flex-col gap-0.5">
				<a
					href="/recap/morning-letter"
					class="flex items-center gap-2 px-3 py-2 text-sm rounded-md text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--surface-hover)] transition-colors"
				>
					<Newspaper class="h-4 w-4" />
					Morning Letter
				</a>
				{#if digest.weeklyRecapAvailable}
					<a
						href="/recap"
						class="flex items-center gap-2 px-3 py-2 text-sm rounded-md text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--surface-hover)] transition-colors"
					>
						<CalendarRange class="h-4 w-4" />
						View Recap
					</a>
				{/if}
				{#if digest.eveningPulseAvailable}
					<a
						href="/recap/evening-pulse"
						class="flex items-center gap-2 px-3 py-2 text-sm rounded-md text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--surface-hover)] transition-colors"
					>
						<Activity class="h-4 w-4" />
						Evening Pulse
					</a>
				{/if}
				<a
					href="/augur"
					class="flex items-center gap-2 px-3 py-2 text-sm rounded-md text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--surface-hover)] transition-colors"
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
					class="flex items-center gap-2 px-3 py-2 text-sm rounded-md text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--surface-hover)] transition-colors"
				>
					<Shuffle class="h-4 w-4" />
					Tag Trail
				</a>
				<a
					href="/feeds/tag-verse"
					class="flex items-center gap-2 px-3 py-2 text-sm rounded-md text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--surface-hover)] transition-colors"
				>
					<Orbit class="h-4 w-4" />
					Tag Verse
				</a>
			</div>
		</div>
	</aside>
{/if}
