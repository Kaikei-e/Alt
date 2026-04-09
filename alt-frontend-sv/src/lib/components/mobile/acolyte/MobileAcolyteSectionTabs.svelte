<script lang="ts">
import type { AcolyteSection } from "$lib/connect/acolyte";

interface Props {
	sections: AcolyteSection[];
	activeSection: string;
	onSelect: (sectionKey: string) => void;
}

const { sections, activeSection, onSelect }: Props = $props();
</script>

<div class="relative">
	<nav
		class="flex gap-0 border-b border-[var(--surface-border,#c8c8c8)] overflow-x-auto [-webkit-overflow-scrolling:touch] [scrollbar-width:none] [&::-webkit-scrollbar]:hidden snap-x snap-mandatory"
		aria-label="Report sections"
	>
		{#each sections as sec}
			{@const isActive = activeSection === sec.sectionKey}
			<button
				class="flex items-baseline gap-1 px-3 min-h-[44px] border-none bg-transparent font-[var(--font-body)] text-[0.8rem] whitespace-nowrap snap-start transition-colors duration-150 border-b-2 shrink-0"
				class:border-b-[var(--alt-charcoal,#1a1a1a)]={isActive}
				class:text-[var(--alt-charcoal,#1a1a1a)]={isActive}
				class:border-b-transparent={!isActive}
				class:text-[var(--alt-ash,#999)]={!isActive}
				data-testid="section-tab-{sec.sectionKey}"
				data-active={isActive}
				onclick={() => onSelect(sec.sectionKey)}
			>
				<span class="capitalize">{sec.sectionKey.replace(/_/g, " ")}</span>
				<span class="font-[var(--font-mono)] text-[0.6rem] text-[var(--alt-ash,#999)]">
					v{sec.currentVersion}
				</span>
			</button>
		{/each}
	</nav>
	<!-- Scroll fade hint -->
	<div
		class="absolute right-0 top-0 bottom-0 w-6 pointer-events-none"
		style="background: linear-gradient(to left, var(--surface-bg, #faf9f7), transparent);"
	></div>
</div>
