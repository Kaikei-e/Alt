<script lang="ts">
import { getVisibleSections } from "./menu-page";

let {
	isAdmin = false,
}: {
	isAdmin?: boolean;
} = $props();

const sections = $derived(getVisibleSections(isAdmin));
</script>

<div
	class="min-h-screen pb-[calc(2.75rem+env(safe-area-inset-bottom,0px))]"
	style="background: var(--app-bg);"
>
	<header class="px-5 pt-4 pb-2">
		<h1
			class="font-[var(--font-display)] text-[22px] font-bold leading-tight text-[var(--text-primary)]"
		>
			Menu
		</h1>
	</header>

	<div class="flex flex-col gap-6 px-5 pt-2 pb-6">
		{#each sections as section}
			<section>
				<h2
					class="mb-3 font-[var(--font-body)] text-[11px] font-semibold uppercase tracking-widest text-[var(--text-secondary)]"
				>
					{section.title}
				</h2>
				<div class="grid grid-cols-3 gap-3">
					{#each section.items as item}
						<a
							href={item.href}
							class="flex min-h-[72px] flex-col items-center justify-center gap-1.5 rounded-xl border border-[var(--surface-border)] bg-[var(--surface-bg)] p-3 transition-colors active:bg-[var(--surface-hover)]"
						>
							<item.icon
								size={24}
								strokeWidth={1.8}
								class="text-[var(--text-secondary)]"
							/>
							<span
								class="text-center font-[var(--font-body)] text-xs leading-tight text-[var(--text-primary)]"
							>
								{item.label}
							</span>
							{#if item.badge}
								<span
									class="rounded-full bg-[var(--badge-gray-bg)] border border-[var(--badge-gray-border)] px-1.5 py-0.5 text-[10px] leading-none text-[var(--badge-gray-text)]"
								>
									{item.badge}
								</span>
							{/if}
						</a>
					{/each}
				</div>
			</section>
		{/each}
	</div>
</div>
