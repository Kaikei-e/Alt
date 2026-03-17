<script lang="ts">
import type { FeatureFlagsConfigData } from "$lib/connect/knowledge_home_admin";
import { CircleCheck, CircleX } from "@lucide/svelte";

let { flags }: { flags: FeatureFlagsConfigData | null } = $props();

const flagItems = $derived(
	flags
		? [
				{ label: "Home Page", enabled: flags.enableHomePage },
				{ label: "Tracking", enabled: flags.enableTracking },
				{ label: "Projection V2", enabled: flags.enableProjectionV2 },
			]
		: [],
);
</script>

<div class="flex flex-col gap-3">
	<h3 class="text-sm font-semibold" style="color: var(--text-primary);">
		Feature Flags
	</h3>

	{#if !flags}
		<p class="text-xs" style="color: var(--text-secondary);">Loading...</p>
	{:else}
		<div class="flex flex-col gap-2">
			{#each flagItems as item}
				<div
					class="flex items-center justify-between rounded-lg border-2 px-3 py-2"
					style="background: var(--surface-bg); border-color: var(--border-primary);"
				>
					<span class="text-sm" style="color: var(--text-primary);">
						{item.label}
					</span>
					{#if item.enabled}
						<CircleCheck size={16} style="color: var(--accent-green, #22c55e);" />
					{:else}
						<CircleX size={16} style="color: var(--text-secondary);" />
					{/if}
				</div>
			{/each}
			<div
				class="flex items-center justify-between rounded-lg border-2 px-3 py-2"
				style="background: var(--surface-bg); border-color: var(--border-primary);"
			>
				<span class="text-sm" style="color: var(--text-primary);">
					Rollout %
				</span>
				<span class="text-sm font-mono font-bold" style="color: var(--text-primary);">
					{flags.rolloutPercentage}%
				</span>
			</div>
		</div>
	{/if}
</div>
