<script lang="ts">
import type { GenreProgressInfo } from "$lib/schema/dashboard";
import { filterGenreProgress } from "$lib/utils/genreProgress";
import { StatusBadge } from "$lib/components/desktop/recap/job-status";
import { Folder, Layers } from "@lucide/svelte";

interface Props {
	genreProgress: Record<string, GenreProgressInfo>;
}

let { genreProgress }: Props = $props();

const genres = $derived(filterGenreProgress(genreProgress));
</script>

{#if genres.length > 0}
	<div class="mt-4" data-testid="mobile-genre-progress-grid">
		<h4 class="text-sm font-semibold mb-2" style="color: var(--text-muted);">
			Genre Progress
		</h4>
		<div class="grid grid-cols-2 gap-2">
			{#each genres as [genre, info]}
				<div
					class="flex items-center justify-between p-2 rounded-lg border"
					style="background: var(--surface-bg); border-color: var(--surface-border);"
				>
					<div class="flex items-center gap-1.5 min-w-0">
						<Folder class="w-3 h-3 flex-shrink-0" style="color: var(--text-muted);" />
						<span
							class="text-xs font-medium truncate"
							style="color: var(--text-primary);"
						>
							{genre}
						</span>
					</div>
					<div class="flex items-center gap-1 flex-shrink-0">
						{#if info.cluster_count !== null}
							<span
								class="text-xs flex items-center gap-0.5"
								style="color: var(--text-muted);"
							>
								<Layers class="w-2.5 h-2.5" />
								{info.cluster_count}
							</span>
						{/if}
						<StatusBadge status={info.status} size="sm" />
					</div>
				</div>
			{/each}
		</div>
	</div>
{/if}
