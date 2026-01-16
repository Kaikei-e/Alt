<script lang="ts">
	import type { GenreProgressInfo } from "$lib/schema/dashboard";
	import StatusBadge from "./StatusBadge.svelte";
	import { Folder, Layers } from "@lucide/svelte";

	interface Props {
		genreProgress: Record<string, GenreProgressInfo>;
	}

	let { genreProgress }: Props = $props();

	const genres = $derived(Object.entries(genreProgress).sort(([a], [b]) => a.localeCompare(b)));
</script>

{#if genres.length > 0}
	<div class="space-y-2">
		<h4 class="text-sm font-semibold" style="color: var(--text-muted);">
			Genre Progress
		</h4>
		<div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-2">
			{#each genres as [genre, info]}
				<div
					class="flex items-center justify-between p-3 rounded-lg border"
					style="background: var(--surface-bg); border-color: var(--surface-border);"
				>
					<div class="flex items-center gap-2">
						<Folder class="w-4 h-4" style="color: var(--text-muted);" />
						<span class="text-sm font-medium" style="color: var(--text-primary);">
							{genre}
						</span>
					</div>
					<div class="flex items-center gap-2">
						{#if info.cluster_count !== null}
							<span
								class="text-xs flex items-center gap-1"
								style="color: var(--text-muted);"
							>
								<Layers class="w-3 h-3" />
								{info.cluster_count}
							</span>
						{/if}
						<StatusBadge status={info.status} size="sm" />
					</div>
				</div>
			{/each}
		</div>
	</div>
{:else}
	<p class="text-sm" style="color: var(--text-muted);">
		No genre progress data available.
	</p>
{/if}
