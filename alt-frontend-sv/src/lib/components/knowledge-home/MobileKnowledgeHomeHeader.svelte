<script lang="ts">
	import { MoreHorizontal } from "@lucide/svelte";
	import {
		type ServiceQuality,
		getQualityDotClass,
		getQualityLabel,
	} from "./mobile-header";

	let {
		serviceQuality = "full",
		onMoreClick,
	}: {
		serviceQuality?: ServiceQuality;
		onMoreClick?: () => void;
	} = $props();

	let isCompact = $state(false);

	$effect(() => {
		function handleScroll() {
			isCompact = window.scrollY > 100;
		}
		window.addEventListener("scroll", handleScroll, { passive: true });
		return () => window.removeEventListener("scroll", handleScroll);
	});
</script>

<header
	class="z-40 bg-[var(--nav-bg)] px-4 transition-all duration-200 ease-out"
	class:sticky={isCompact}
	class:top-0={isCompact}
	class:border-b={isCompact}
	class:border-[var(--divider-rule)]={isCompact}
	class:pt-3={!isCompact}
	class:pb-2={!isCompact}
	class:py-2={isCompact}
>
	<div class="flex items-center justify-between">
		<div>
			<h1
				class="font-[var(--font-display)] font-bold text-[var(--text-primary)] transition-all duration-200"
				class:text-[22px]={!isCompact}
				class:leading-tight={!isCompact}
				class:text-[17px]={isCompact}
			>
				Knowledge Home
			</h1>
			{#if !isCompact}
				<p class="mt-0.5 font-[var(--font-body)] text-[13px] text-[var(--text-secondary)]">
					Today's knowledge starting point
				</p>
			{/if}
		</div>

		<div class="flex items-center gap-2">
			{#if serviceQuality !== "full"}
				<span
					class="h-2 w-2 rounded-full {getQualityDotClass(serviceQuality)}"
					aria-label={getQualityLabel(serviceQuality)}
					role="status"
				></span>
			{/if}
			<button
				type="button"
				class="rounded-lg p-2 text-[var(--text-secondary)] transition-colors active:bg-[var(--surface-hover)]"
				aria-label="More options"
				onclick={onMoreClick}
			>
				<MoreHorizontal size={24} />
			</button>
		</div>
	</div>
</header>
