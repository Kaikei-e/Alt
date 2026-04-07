<script lang="ts">
import { NAV_TABS, shouldShowBottomNav, getActiveTabIndex } from "./bottom-nav";

let { pathname }: { pathname: string } = $props();

const show = $derived(shouldShowBottomNav(pathname));
const activeIndex = $derived(getActiveTabIndex(pathname));
</script>

{#if show}
	<nav
		aria-label="Main navigation"
		class="fixed bottom-0 left-0 right-0 z-50 grid grid-cols-5 border-t border-[var(--nav-border-top)] bg-[var(--nav-bg)]"
		style="padding-bottom: env(safe-area-inset-bottom, 0px); touch-action: manipulation; box-shadow: 0 50px 0 0 var(--nav-bg);"
	>
		{#each NAV_TABS as tab, i}
			{@const isActive = activeIndex === i}
			<a
				href={tab.href}
				role="tab"
				aria-selected={isActive}
				aria-label={tab.label}
				data-active={isActive}
				class="flex h-11 flex-col items-center justify-center gap-px"
			>
				<tab.icon
					size={18}
					strokeWidth={isActive ? 2.5 : 1.8}
					class={isActive
						? "text-[var(--nav-active-icon)]"
						: "text-[var(--nav-inactive-icon)]"}
				/>
				<span
					class="text-[10px] leading-none"
					class:font-semibold={isActive}
					class:text-[var(--nav-label-active)]={isActive}
					class:font-normal={!isActive}
					class:text-[var(--nav-label-inactive)]={!isActive}
				>
					{tab.label}
				</span>
			</a>
		{/each}
	</nav>
{/if}
