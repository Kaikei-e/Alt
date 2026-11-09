<script lang="ts">
import { AlertTriangle } from "@lucide/svelte";
import { Button } from "#lib/components/ui/button/index.js";

interface Props {
	error: Error;
	onRetry?: () => void;
	isRetrying?: boolean;
}

const { error, onRetry, isRetrying = false }: Props = $props();
</script>

<div class="flex flex-col items-center justify-center min-h-[50dvh] p-6">
	<div
		class="p-6 border text-center max-w-sm w-full"
		style="background: var(--surface-bg); border-color: var(--surface-border); border-top: 3px solid var(--alt-error);"
	>
		<div class="flex justify-center mb-4" aria-hidden="true">
			<AlertTriangle class="h-8 w-8" style="color: var(--alt-error);" />
		</div>
		<h2
			class="text-base font-bold mb-2"
			style="color: var(--alt-error); font-family: var(--font-display);"
		>
			Error loading Evening Pulse
		</h2>
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
				class="px-5 py-2 rounded-none font-mono uppercase tracking-wider text-xs disabled:opacity-50"
				style="background: var(--alt-charcoal); color: var(--surface-bg); border: 1px solid var(--alt-charcoal);"
			>
				{isRetrying ? "Retrying..." : "Retry"}
			</Button>
		{/if}
	</div>
</div>
