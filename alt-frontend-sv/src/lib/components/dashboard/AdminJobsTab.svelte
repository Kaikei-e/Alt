<script lang="ts">
import { getJobs } from "$lib/api/client/dashboard";
import type { AdminJob } from "$lib/schema/dashboard";

interface Props {
	windowSeconds: number;
}

let { windowSeconds }: Props = $props();

let jobs = $state<AdminJob[]>([]);
let loading = $state(true);
let error = $state<string | null>(null);

$effect(() => {
	loadData();
});

async function loadData() {
	loading = true;
	error = null;
	try {
		jobs = await getJobs(windowSeconds, 200);
	} catch (e) {
		error = e instanceof Error ? e.message : String(e);
		console.error("Failed to load admin jobs:", e);
	} finally {
		loading = false;
	}
}

const runningCount = $derived(
	jobs.filter((j) => j.status === "running").length,
);
const failedCount = $derived(jobs.filter((j) => j.status === "failed").length);
const succeededCount = $derived(
	jobs.filter((j) => j.status === "succeeded" || j.status === "partial").length,
);

function getDuration(job: AdminJob): string {
	if (!job.finished_at) return "N/A";
	const start = new Date(job.started_at).getTime();
	const end = new Date(job.finished_at).getTime();
	const seconds = Math.round((end - start) / 1000);
	if (seconds < 60) return `${seconds}s`;
	const minutes = Math.floor(seconds / 60);
	const remainingSeconds = seconds % 60;
	return `${minutes}m ${remainingSeconds}s`;
}
</script>

<div>
	<h2 class="text-2xl font-bold mb-4" style="color: var(--text-primary);">
		Admin Jobs (Graph / Learning)
	</h2>

	{#if loading}
		<div class="p-8 text-center" style="color: var(--text-muted);">
			Loading...
		</div>
	{:else if error}
		<div class="p-8 text-center" style="color: var(--alt-error);">
			Error: {error}
		</div>
	{:else if jobs.length === 0}
		<div class="p-8 text-center" style="color: var(--text-muted);">
			No admin jobs found.
		</div>
	{:else}
		<div class="grid grid-cols-1 md:grid-cols-3 gap-4 mb-6">
			<div
				class="p-4 border"
				style="
					background: var(--surface-bg);
					border-color: var(--surface-border);
					box-shadow: var(--shadow-sm);
				"
			>
				<div class="text-sm mb-1" style="color: var(--text-muted);">
					Running
				</div>
				<div class="text-2xl font-bold" style="color: var(--text-primary);">
					{runningCount}
				</div>
			</div>
			<div
				class="p-4 border"
				style="
					background: var(--surface-bg);
					border-color: var(--surface-border);
					box-shadow: var(--shadow-sm);
				"
			>
				<div class="text-sm mb-1" style="color: var(--text-muted);">
					Succeeded/Partial
				</div>
				<div class="text-2xl font-bold" style="color: var(--text-primary);">
					{succeededCount}
				</div>
			</div>
			<div
				class="p-4 border"
				style="
					background: var(--surface-bg);
					border-color: var(--surface-border);
					box-shadow: var(--shadow-sm);
				"
			>
				<div class="text-sm mb-1" style="color: var(--text-muted);">
					Failed
				</div>
				<div class="text-2xl font-bold" style="color: var(--text-primary);">
					{failedCount}
				</div>
			</div>
		</div>

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
							Kind
						</th>
						<th
							class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider"
							style="color: var(--text-muted);"
						>
							Status
						</th>
						<th
							class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider"
							style="color: var(--text-muted);"
						>
							Started At
						</th>
						<th
							class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider"
							style="color: var(--text-muted);"
						>
							Duration
						</th>
						<th
							class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider"
							style="color: var(--text-muted);"
						>
							Error
						</th>
					</tr>
				</thead>
				<tbody style="border-top: 1px solid var(--surface-border);">
					{#each jobs as job}
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
								{job.job_id}
							</td>
							<td
								class="px-6 py-4 whitespace-nowrap text-sm"
								style="color: var(--text-primary);"
							>
								{job.kind}
							</td>
							<td
								class="px-6 py-4 whitespace-nowrap text-sm"
								style="color: {job.status === 'succeeded' || job.status === 'partial' ? 'var(--alt-success)' : job.status === 'failed' ? 'var(--alt-error)' : 'var(--text-primary)'};"
							>
								{job.status}
							</td>
							<td
								class="px-6 py-4 whitespace-nowrap text-sm"
								style="color: var(--text-primary);"
							>
								{new Date(job.started_at).toLocaleString()}
							</td>
							<td
								class="px-6 py-4 whitespace-nowrap text-sm"
								style="color: var(--text-primary);"
							>
								{getDuration(job)}
							</td>
							<td
								class="px-6 py-4 text-sm"
								style="color: {job.error ? 'var(--alt-error)' : 'var(--text-muted)'};"
							>
								{job.error || "—"}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>

