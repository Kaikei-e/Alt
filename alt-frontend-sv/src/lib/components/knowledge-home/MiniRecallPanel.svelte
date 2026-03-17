<script lang="ts">
import { CalendarRange, Activity } from "@lucide/svelte";
import type { TodayDigestData } from "$lib/connect/knowledge_home";

interface Props {
	digest: TodayDigestData | null;
}

const { digest }: Props = $props();
</script>

{#if digest}
	<aside
		class="border rounded-lg p-4 bg-[var(--surface-bg)] border-[var(--surface-border)]"
	>
		<h3 class="text-xs font-semibold text-[var(--text-secondary)] uppercase tracking-wider mb-3">
			Quick Access
		</h3>

		<!-- Top Tags -->
		{#if digest.topTags.length > 0}
			<div class="mb-4">
				<p class="text-xs text-[var(--text-secondary)] mb-2">Top Tags</p>
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

		<!-- Shortcuts -->
		<div class="flex flex-col gap-1">
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
		</div>
	</aside>
{/if}
