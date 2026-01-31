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
		class="p-8 border-2 text-center max-w-md w-full"
		style="background: white; border-color: var(--destructive);"
	>
		<div class="text-4xl mb-4" aria-hidden="true">
			&#9888;
		</div>
		<h2
			class="text-lg font-semibold mb-2"
			style="color: var(--destructive);"
		>
			Error loading Evening Pulse
		</h2>
		<p
			class="text-sm mb-6"
			style="color: var(--text-secondary);"
		>
			{error.message}
		</p>
		{#if onRetry}
			<Button
				onclick={onRetry}
				disabled={isRetrying}
				class="px-6 py-2 disabled:opacity-50"
				style="background: var(--alt-primary); color: white;"
			>
				{isRetrying ? "Retrying..." : "Retry"}
			</Button>
		{/if}
	</div>
</div>
