<script lang="ts">
import type { RetentionRunResponse } from "$lib/types/sovereign-admin";

interface Props {
	result: RetentionRunResponse | null;
}

let { result }: Props = $props();

const statusColor = (status: string) => {
	switch (status) {
		case "exported":
			return "var(--accent-green, #22c55e)";
		case "dry_run":
		case "planned":
			return "var(--accent-blue, #3b82f6)";
		case "failed":
			return "var(--accent-red, #ef4444)";
		default:
			return "var(--text-secondary)";
	}
};
</script>

{#if result}
	<div class="flex flex-col gap-3">
		<div class="flex items-center gap-2">
			<h3 class="text-sm font-semibold" style="color: var(--text-primary);">
				Retention Run Result
			</h3>
			<span
				class="inline-block rounded px-2 py-0.5 text-xs font-medium"
				style="background: {result.dry_run ? 'var(--accent-blue, #3b82f6)' : 'var(--accent-green, #22c55e)'}; color: #fff;"
			>
				{result.dry_run ? "Dry Run" : "Live"}
			</span>
		</div>

		{#if result.error}
			<div
				class="rounded-lg border px-4 py-2 text-sm"
				style="background: var(--error-bg, #fee2e2); border-color: var(--error-border, #ef4444); color: var(--error-text, #991b1b);"
			>
				{result.error}
			</div>
		{/if}

		{#if result.actions.length === 0}
			<p class="text-xs" style="color: var(--text-secondary);">No actions taken.</p>
		{:else}
			<div
				class="overflow-x-auto rounded-lg border-2"
				style="border-color: var(--border-primary);"
			>
				<table class="w-full text-xs">
					<thead>
						<tr style="background: var(--surface-bg);">
							<th class="px-3 py-2 text-left">Status</th>
							<th class="px-3 py-2 text-left">Action</th>
							<th class="px-3 py-2 text-left">Table</th>
							<th class="px-3 py-2 text-left">Partition</th>
							<th class="px-3 py-2 text-right">Rows</th>
							<th class="px-3 py-2 text-left">Path</th>
						</tr>
					</thead>
					<tbody>
						{#each result.actions as action}
							<tr class="border-t" style="border-color: var(--border-primary);">
								<td class="px-3 py-2">
									<span
										class="inline-block rounded px-2 py-0.5 text-white text-xs font-medium"
										style="background: {statusColor(action.status)};"
									>
										{action.status}
									</span>
								</td>
								<td class="px-3 py-2">{action.action}</td>
								<td class="px-3 py-2 font-mono">{action.table}</td>
								<td class="px-3 py-2 font-mono">{action.partition}</td>
								<td class="px-3 py-2 text-right tabular-nums">{action.rows.toLocaleString()}</td>
								<td class="px-3 py-2 max-w-48 truncate font-mono" title={action.path ?? ""}>
									{action.path ?? "—"}
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	</div>
{/if}
