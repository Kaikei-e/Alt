<script lang="ts">
import type { ToastItem } from "$lib/stores/toast.svelte";

interface Props {
	items: ToastItem[];
	onDismiss: (id: string) => void;
}

const { items, onDismiss }: Props = $props();

const kindClass: Record<ToastItem["kind"], string> = {
	success: "toast-kind-success",
	error: "toast-kind-error",
	info: "toast-kind-info",
};
</script>

{#if items.length > 0}
	<div class="pointer-events-none fixed bottom-4 right-4 z-50 flex w-80 flex-col gap-2">
		{#each items as item (item.id)}
			<div
				class="pointer-events-auto animate-slide-in-right border px-3 py-2 text-sm {kindClass[item.kind]}"
			>
				<div class="flex items-start justify-between gap-3">
					<p>{item.message}</p>
					<button
						type="button"
						class="text-xs opacity-80 transition-opacity hover:opacity-100"
						onclick={() => onDismiss(item.id)}
						aria-label="Dismiss notification"
					>
						Close
					</button>
				</div>
			</div>
		{/each}
	</div>
{/if}

<style>
	:global(.toast-kind-success) {
		color: var(--alt-success);
		background: color-mix(in srgb, var(--alt-success) 6%, var(--surface-bg));
		border-color: color-mix(in srgb, var(--alt-success) 30%, transparent);
	}
	:global(.toast-kind-error) {
		color: var(--alt-error);
		background: color-mix(in srgb, var(--alt-error) 6%, var(--surface-bg));
		border-color: color-mix(in srgb, var(--alt-error) 30%, transparent);
	}
	:global(.toast-kind-info) {
		color: var(--alt-charcoal);
		background: var(--surface-bg);
		border-color: var(--surface-border);
	}
</style>
