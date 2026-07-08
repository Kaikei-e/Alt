<script lang="ts">
import { onMount } from "svelte";
import { getOverview } from "$lib/api/client/dashboard";
import type { RecentActivity, TimeWindow } from "$lib/schema/dashboard";
import { TIME_WINDOWS } from "$lib/schema/dashboard";

interface Props {
	windowSeconds: number;
}

let { windowSeconds }: Props = $props();

let activities = $state<RecentActivity[]>([]);
let loading = $state(true);
let error = $state<string | null>(null);
let requestId = 0;

$effect(() => {
	loadData();
});

async function loadData() {
	const currentRequest = ++requestId;
	loading = true;
	error = null;
	try {
		const result = await getOverview(windowSeconds, 200);
		if (currentRequest !== requestId) return;
		activities = result;
	} catch (e) {
		if (currentRequest !== requestId) return;
		error = e instanceof Error ? e.message : String(e);
		console.error("Failed to load overview:", e);
	} finally {
		if (currentRequest === requestId) {
			loading = false;
		}
	}
}
</script>

<div>
	<h2 class="text-2xl font-bold mb-4" style="color: var(--text-primary);">
		Recent Activity
	</h2>

	{#if loading}
		<div class="p-8 text-center" style="color: var(--text-muted);">
			Loading...
		</div>
	{:else if error}
		<div class="p-8 text-center" style="color: var(--alt-error);">
			Error: {error}
		</div>
	{:else if activities.length === 0}
		<div class="p-8 text-center" style="color: var(--text-muted);">
			No recent activity found.
		</div>
	{:else}
		<div
			class="border overflow-hidden"
			style="
				background: var(--surface-bg);
				border-color: var(--surface-border);
				box-shadow: var(--shadow-sm);
			"
		>
			<table class="w-full">
				<thead
					style="
						background: var(--surface-hover);
						border-bottom: 1px solid var(--surface-border);
					"
				>
					<tr>
						<th
							class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider"
							style="color: var(--text-muted);"
						>
							Job ID
						</th>
						<th
							class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider"
							style="color: var(--text-muted);"
						>
							Metric Type
						</th>
						<th
							class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider"
							style="color: var(--text-muted);"
						>
							Timestamp
						</th>
					</tr>
				</thead>
				<tbody style="border-top: 1px solid var(--surface-border);">
					{#each activities as activity}
						<tr
							style="
								border-bottom: 1px solid var(--surface-border);
								transition: background 0.2s;
							"
							onmouseenter={(e) => {
								e.currentTarget.style.background = "var(--surface-hover)";
							}}
							onmouseleave={(e) => {
								e.currentTarget.style.background = "transparent";
							}}
						>
							<td
								class="px-6 py-4 whitespace-nowrap text-sm"
								style="color: var(--text-primary);"
							>
								{activity.job_id || "N/A"}
							</td>
							<td
								class="px-6 py-4 whitespace-nowrap text-sm"
								style="color: var(--text-primary);"
							>
								{activity.metric_type}
							</td>
							<td
								class="px-6 py-4 whitespace-nowrap text-sm"
								style="color: var(--text-primary);"
							>
								{new Date(activity.timestamp).toLocaleString()}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>

