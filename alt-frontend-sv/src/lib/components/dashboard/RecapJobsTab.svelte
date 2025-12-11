<script lang="ts">
	import { onMount } from 'svelte';
	import { getRecapJobs } from '$lib/api/client/dashboard';
	import type { RecapJob } from '$lib/schema/dashboard';

	interface Props {
		windowSeconds: number;
	}

	let { windowSeconds } = $props();

	let jobs = $state<RecapJob[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);

	async function load() {
		loading = true;
		error = null;
		try {
			// Pass windowSeconds and limit (default 200)
			jobs = await getRecapJobs(fetch, windowSeconds, 200);
		} catch (e: any) {
			error = e.message;
			console.error("Failed to load recap jobs", e);
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		// Reload when windowSeconds changes
		if (windowSeconds) {
			load();
		}
	});

	onMount(() => {
		load();
	});
</script>

<div class="overflow-x-auto">
	<div class="flex justify-between items-center mb-4">
		<h3 class="text-lg font-medium" style="color: var(--text-primary)">Recap Jobs History</h3>
		<button
			onclick={load}
			class="px-3 py-1 text-sm rounded bg-blue-600 text-white hover:bg-blue-700 transition"
		>
			Refresh
		</button>
	</div>

	{#if loading}
		<div class="p-4 text-center" style="color: var(--text-muted)">Loading jobs...</div>
	{:else if error}
		<div class="p-4 text-center text-red-500">Error: {error}</div>
	{:else if jobs.length === 0}
		<div class="p-4 text-center" style="color: var(--text-muted)">No jobs found in this period.</div>
	{:else}
		<table class="min-w-full divide-y" style="border-color: var(--surface-border)">
			<thead>
				<tr>
					{#each ['Job ID', 'Status', 'Last Stage', 'Kicked At', 'Updated At'] as header}
						<th class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider" style="color: var(--text-muted)">
							{header}
						</th>
					{/each}
				</tr>
			</thead>
			<tbody class="divide-y" style="border-color: var(--surface-border)">
				{#each jobs as job}
					<tr class="hover:bg-gray-50 transition" style="color: var(--text-primary)">
						<td class="px-6 py-4 whitespace-nowrap text-sm font-mono">{job.job_id}</td>
						<td class="px-6 py-4 whitespace-nowrap text-sm">
							<span class={`px-2 inline-flex text-xs leading-5 font-semibold rounded-full
								${job.status === 'completed' ? 'bg-green-100 text-green-800' :
								  job.status === 'failed' ? 'bg-red-100 text-red-800' :
								  'bg-yellow-100 text-yellow-800'}`}>
								{job.status}
							</span>
						</td>
						<td class="px-6 py-4 whitespace-nowrap text-sm">{job.last_stage || '-'}</td>
						<td class="px-6 py-4 whitespace-nowrap text-sm">{new Date(job.kicked_at).toLocaleString()}</td>
						<td class="px-6 py-4 whitespace-nowrap text-sm">{new Date(job.updated_at).toLocaleString()}</td>
					</tr>
				{/each}
			</tbody>
		</table>
	{/if}
</div>
