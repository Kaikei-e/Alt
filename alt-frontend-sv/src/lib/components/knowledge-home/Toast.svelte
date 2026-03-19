<script lang="ts">
import type { ToastItem } from "$lib/stores/toast.svelte";

interface Props {
	items: ToastItem[];
	onDismiss: (id: string) => void;
}

const { items, onDismiss }: Props = $props();

const kindClass: Record<ToastItem["kind"], string> = {
	success:
		"border-emerald-400/30 bg-emerald-400/10 text-emerald-200",
	error: "border-red-400/30 bg-red-400/10 text-red-200",
	info: "border-[var(--surface-border)] bg-[var(--surface-bg)] text-[var(--text-primary)]",
};
</script>

{#if items.length > 0}
	<div class="pointer-events-none fixed bottom-4 right-4 z-50 flex w-80 flex-col gap-2">
		{#each items as item (item.id)}
			<div
				class="pointer-events-auto rounded-lg border px-3 py-2 text-sm shadow-lg {kindClass[item.kind]}"
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
