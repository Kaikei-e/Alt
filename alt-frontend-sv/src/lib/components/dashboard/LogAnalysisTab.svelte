<script lang="ts">
	import { getLogs } from "$lib/api/client/dashboard";
	import type { LogError } from "$lib/schema/dashboard";

	interface Props {
		windowSeconds: number;
	}

	let { windowSeconds }: Props = $props();

	let logs = $state<LogError[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);
	let selectedErrorType = $state<string>("All");

	$effect(() => {
		loadData();
	});

	async function loadData() {
		loading = true;
		error = null;
		try {
			logs = await getLogs(windowSeconds, 2000);
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
			console.error("Failed to load logs:", e);
		} finally {
			loading = false;
		}
	}

	const errorTypes = $derived(
		["All", ...Array.from(new Set(logs.map((log) => log.error_type)))],
	);

	const filteredLogs = $derived(
		selectedErrorType === "All"
			? logs
			: logs.filter((log) => log.error_type === selectedErrorType),
	);

	const errorTypeCounts = $derived(() => {
		const counts: Record<string, number> = {};
		for (const log of logs) {
			counts[log.error_type] = (counts[log.error_type] || 0) + 1;
		}
		return counts;
	});
</script>

<div>
	<h2 class="text-2xl font-bold mb-4" style="color: var(--text-primary);">
		Log Analysis
	</h2>

	{#if loading}
		<div class="p-8 text-center" style="color: var(--text-muted);">
			Loading...
		</div>
	{:else if error}
		<div class="p-8 text-center" style="color: var(--alt-error);">
			Error: {error}
		</div>
	{:else if logs.length === 0}
		<div class="p-8 text-center" style="color: var(--text-muted);">
			No log data available.
		</div>
	{:else}
		<div class="grid grid-cols-1 md:grid-cols-2 gap-4 mb-6">
			<div
				class="p-4 border"
				style="
					background: var(--surface-bg);
					border-color: var(--surface-border);
					box-shadow: var(--shadow-sm);
				"
			>
				<div class="text-sm mb-1" style="color: var(--text-muted);">
					Total Recorded Errors
				</div>
				<div class="text-2xl font-bold" style="color: var(--text-primary);">
					{logs.length}
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
					Unique Error Types
				</div>
				<div class="text-2xl font-bold" style="color: var(--text-primary);">
					{Object.keys(errorTypeCounts).length}
				</div>
			</div>
		</div>

		<div class="mb-6">
			<label
				for="error-type-select"
				class="block text-sm font-medium mb-2"
				style="color: var(--text-primary);"
			>
				Filter by Error Type
			</label>
			<select
				id="error-type-select"
				bind:value={selectedErrorType}
				class="px-4 py-2 border rounded"
				style="
					background: var(--surface-bg);
					border-color: var(--surface-border);
					color: var(--text-primary);
				"
			>
				{#each errorTypes as type}
					<option value={type}>{type}</option>
				{/each}
			</select>
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
							Timestamp
						</th>
						<th
							class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider"
							style="color: var(--text-muted);"
						>
							Error Type
						</th>
						<th
							class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider"
							style="color: var(--text-muted);"
						>
							Message
						</th>
						<th
							class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider"
							style="color: var(--text-muted);"
						>
							Service
						</th>
					</tr>
				</thead>
				<tbody style="border-top: 1px solid var(--surface-border);">
					{#each filteredLogs.slice(0, 100) as log}
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
								{new Date(log.timestamp).toLocaleString()}
							</td>
							<td
								class="px-6 py-4 whitespace-nowrap text-sm"
								style="color: var(--text-primary);"
							>
								{log.error_type}
							</td>
							<td
								class="px-6 py-4 text-sm"
								style="color: var(--text-primary);"
							>
								{log.error_message || log.raw_line || "N/A"}
							</td>
							<td
								class="px-6 py-4 whitespace-nowrap text-sm"
								style="color: var(--text-primary);"
							>
								{log.service || "N/A"}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>

