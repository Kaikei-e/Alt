<script lang="ts">
	import * as Sheet from "$lib/components/ui/sheet";
	import { getVisibleItems } from "./more-sheet";

	let {
		open = $bindable(false),
		isAdmin = false,
	}: {
		open: boolean;
		isAdmin?: boolean;
	} = $props();

	const items = $derived(getVisibleItems(isAdmin));
</script>

<Sheet.Root bind:open>
	<Sheet.Content side="bottom" class="rounded-t-2xl border-t border-[var(--surface-border)] bg-[var(--surface-bg)] max-h-[70vh]">
		<Sheet.Header class="sr-only">
			<Sheet.Title>More</Sheet.Title>
		</Sheet.Header>

		<!-- Drag handle -->
		<div class="mx-auto mt-3 mb-4 h-1 w-8 rounded-full bg-[var(--surface-border)]"></div>

		<div class="flex flex-col pb-[env(safe-area-inset-bottom,0px)]">
			{#each items as item, i}
				<a
					href={item.href}
					class="flex items-center gap-3 px-5 py-3.5 text-sm font-medium text-[var(--text-primary)] transition-colors active:bg-[var(--surface-hover)]"
					class:border-b={i < items.length - 1}
					class:border-[var(--divider-rule)]={i < items.length - 1}
					onclick={() => { open = false; }}
				>
					<item.icon size={20} class="text-[var(--text-secondary)]" />
					<span class="flex-1">{item.label}</span>
					{#if item.badge}
						<span class="rounded-full bg-[var(--badge-gray-bg)] border border-[var(--badge-gray-border)] px-2 py-0.5 text-xs text-[var(--badge-gray-text)]">
							{item.badge}
						</span>
					{/if}
				</a>
			{/each}
		</div>
	</Sheet.Content>
</Sheet.Root>
