<script lang="ts">
import {
	buildTopicsKeyRows,
	type TopicClusterSummary,
} from "./scene/sceneModel.js";

interface Props {
	clusters: readonly TopicClusterSummary[];
	focusedClusterId?: number | null;
	onFocusCluster: (clusterId: number | null) => void;
}

let { clusters, focusedClusterId = null, onFocusCluster }: Props = $props();

const rows = $derived(buildTopicsKeyRows(clusters));

function handleEnter(clusterId: number) {
	onFocusCluster(clusterId);
}

function handleLeave() {
	onFocusCluster(null);
}
</script>

{#if rows.length > 0}
	<section
		aria-label="Topics"
		class="
			absolute right-4 bottom-4 z-10 max-w-[220px] rounded-none border
			border-[var(--surface-border)] bg-[var(--surface-bg)] px-3 py-2
			text-[var(--text-primary)]
		"
	>
		<h2
			class="
				mb-1.5 text-[10px] font-semibold tracking-[0.12em] uppercase
				text-[var(--text-secondary)]
			"
		>
			Topics
		</h2>
		<ul class="flex flex-col gap-1">
			{#each rows as row (row.clusterId)}
				{@const isEmphasized = focusedClusterId === row.clusterId}
				<li>
					<button
						type="button"
						aria-label={row.ariaLabel}
						data-emphasized={isEmphasized ? "true" : "false"}
						onmouseenter={() => handleEnter(row.clusterId)}
						onmouseleave={() => handleLeave()}
						onfocus={() => handleEnter(row.clusterId)}
						onblur={() => handleLeave()}
						class="
							flex w-full items-center gap-2 rounded-none px-1.5 py-0.5
							text-left text-xs text-[var(--text-primary)] transition-colors
							hover:bg-[var(--surface-hover)]
							focus-visible:bg-[var(--surface-hover)] focus-visible:outline-2
							focus-visible:outline-offset-1
							focus-visible:outline-[var(--interactive-text)]
						"
					>
						<span
							aria-hidden="true"
							class="h-2.5 w-2.5 shrink-0 rounded-none"
							style:background-color={row.ink}
						></span>
						<span class="truncate">{row.label}</span>
					</button>
				</li>
			{/each}
		</ul>
	</section>
{/if}
