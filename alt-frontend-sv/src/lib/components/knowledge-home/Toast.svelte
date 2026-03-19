<script lang="ts">
import type { ToastItem } from "$lib/stores/toast.svelte";

interface Props {
	items: ToastItem[];
	onDismiss: (id: string) => void;
}

const { items, onDismiss }: Props = $props();

const kindClass: Record<ToastItem["kind"], string> = {
	success:
		"border-[var(--badge-green-border)] bg-[var(--badge-green-bg)] text-[var(--badge-green-text)]",
	error: "border-[var(--badge-orange-border)] bg-[var(--badge-orange-bg)] text-[var(--badge-orange-text)]",
	info: "border-[var(--surface-border)] bg-[var(--surface-bg)] text-[var(--text-primary)]",
};
</script>

{#if items.length > 0}
	<div class="pointer-events-none fixed bottom-4 right-4 z-50 flex w-80 flex-col gap-2">
		{#each items as item (item.id)}
			<div
				class="pointer-events-auto animate-slide-in-right rounded-xl border px-3 py-2 text-sm shadow-xl {kindClass[item.kind]}"
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
