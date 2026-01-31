<script lang="ts">
import { Button } from "$lib/components/ui/button";

interface Props {
	error: Error;
	onRetry?: () => void;
	isRetrying?: boolean;
}

const { error, onRetry, isRetrying = false }: Props = $props();
</script>

<div class="flex flex-col items-center justify-center min-h-[50vh] p-6">
	<div
		class="p-6 rounded-lg border text-center max-w-sm"
		style="background: var(--surface-bg); border-color: hsl(var(--destructive));"
	>
		<div class="text-3xl mb-4" aria-hidden="true">
			\u26A0
		</div>
		<p
			class="font-semibold mb-2"
			style="color: hsl(var(--destructive));"
		>
			Error loading Evening Pulse
		</p>
		<p
			class="text-sm mb-4"
			style="color: var(--text-secondary);"
		>
			{error.message}
		</p>
		{#if onRetry}
			<Button
				onclick={onRetry}
				disabled={isRetrying}
				class="px-4 py-2 rounded disabled:opacity-50"
				style="background: var(--alt-primary); color: var(--text-primary);"
			>
				{isRetrying ? "Retrying..." : "Retry"}
			</Button>
		{/if}
	</div>
</div>
