<script lang="ts">
import type { TableStorageInfo } from "$lib/types/sovereign-admin";
import AdminMetricCard from "./AdminMetricCard.svelte";

interface Props {
	stats: TableStorageInfo[];
}

let { stats }: Props = $props();

const sizeStatus = (bytes: number): "ok" | "warning" | "error" | "neutral" => {
	if (bytes > 500_000_000) return "warning";
	if (bytes > 1_000_000_000) return "error";
	return "ok";
};
</script>

<div class="flex flex-col gap-3" data-testid="storage-stats-panel">
	<h3 class="text-sm font-semibold" style="color: var(--text-primary);">
		Table Storage
	</h3>

	{#if stats.length === 0}
		<p class="text-xs" style="color: var(--text-secondary);">No storage data available.</p>
	{:else}
		<div class="grid grid-cols-2 gap-3 lg:grid-cols-4">
			{#each stats as stat (stat.table_name)}
				<AdminMetricCard
					label={stat.table_name}
					value={stat.total_size}
					status={sizeStatus(stat.total_bytes)}
				>
					{#snippet icon()}
						{#if stat.is_partitioned}
							<span
								class="inline-block rounded px-1.5 py-0.5 text-[10px] font-medium"
								style="background: var(--accent-blue, #3b82f6); color: #fff;"
							>
								partitioned
							</span>
						{/if}
					{/snippet}
				</AdminMetricCard>
			{/each}
		</div>

		<!-- Detailed table -->
		<div
			class="overflow-x-auto rounded-lg border-2"
			style="border-color: var(--border-primary);"
		>
			<table class="w-full text-xs">
				<thead>
					<tr style="background: var(--surface-bg);">
						<th class="px-3 py-2 text-left">Table</th>
						<th class="px-3 py-2 text-right">Rows</th>
						<th class="px-3 py-2 text-right">Size</th>
						<th class="px-3 py-2 text-center">Partitioned</th>
					</tr>
				</thead>
				<tbody>
					{#each stats as stat (stat.table_name)}
						<tr class="border-t" style="border-color: var(--border-primary);">
							<td class="px-3 py-2 font-mono">{stat.table_name}</td>
							<td class="px-3 py-2 text-right tabular-nums">{stat.row_count.toLocaleString()}</td>
							<td class="px-3 py-2 text-right">{stat.total_size}</td>
							<td class="px-3 py-2 text-center">
								{#if stat.is_partitioned}
									<span style="color: var(--accent-blue, #3b82f6);">Yes</span>
								{:else}
									<span style="color: var(--text-secondary);">No</span>
								{/if}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>
