<script lang="ts">
	import type { RecentJobSummary } from "$lib/schema/dashboard";
	import MobileJobCard from "./MobileJobCard.svelte";
	import { Inbox } from "@lucide/svelte";

	interface Props {
		jobs: RecentJobSummary[];
		onJobSelect: (job: RecentJobSummary) => void;
	}

	let { jobs, onJobSelect }: Props = $props();
</script>

<div class="px-4">
	<h2
		class="text-sm font-semibold mb-3"
		style="color: var(--text-muted);"
	>
		Recent Jobs
	</h2>

	{#if jobs.length === 0}
		<div
			class="p-8 rounded-xl border text-center"
			style="background: var(--surface-bg); border-color: var(--surface-border);"
		>
			<Inbox class="w-8 h-8 mx-auto mb-2" style="color: var(--text-muted);" />
			<p class="text-sm" style="color: var(--text-muted);">
				No jobs found in the selected time window
			</p>
		</div>
	{:else}
		<div class="space-y-3">
			{#each jobs as job (job.job_id)}
				<MobileJobCard {job} onSelect={onJobSelect} />
			{/each}
		</div>
	{/if}
</div>
