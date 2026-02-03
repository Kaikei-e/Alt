<script lang="ts">
import type { Snippet } from "svelte";

let {
	title,
	description = "",
	actions,
}: {
	title: string;
	description?: string;
	actions?: Snippet;
} = $props();
</script>

<header class="mb-6">
	<div class="flex items-center justify-between">
		<div>
			<h1 class="text-2xl font-bold text-[var(--text-primary)]">{title}</h1>
			{#if description}
				<p class="text-sm text-[var(--text-secondary)] mt-1">{description}</p>
			{/if}
		</div>
		{#if actions}
			<div class="flex items-center">
				{@render actions()}
			</div>
		{/if}
	</div>
</header>
