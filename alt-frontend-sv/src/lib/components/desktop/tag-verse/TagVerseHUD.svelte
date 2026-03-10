<script lang="ts">
import { X, FileText, BookOpen } from "@lucide/svelte";
import HUDArticlesTab from "./HUDArticlesTab.svelte";
import HUDRecapsTab from "./HUDRecapsTab.svelte";

interface Props {
	tagName: string;
	articleCount?: number;
	onClose: () => void;
}

let { tagName, articleCount, onClose }: Props = $props();

let activeTab = $state<"articles" | "recaps">("articles");

// Close on Escape key
function handleKeydown(e: KeyboardEvent) {
	if (e.key === "Escape") {
		onClose();
	}
}
</script>

<svelte:window onkeydown={handleKeydown} />

<!-- Backdrop (click to close) -->
<button
	type="button"
	class="fixed inset-0 z-40 bg-transparent"
	onclick={onClose}
	aria-label="Close panel"
></button>

<!-- HUD Panel -->
<div
	class="fixed right-0 top-0 z-50 flex h-full w-[420px] flex-col border-l border-cyan-500/20 bg-[rgba(10,10,30,0.85)] backdrop-blur-xl animate-slide-in"
>
	<!-- Header -->
	<div class="flex items-center justify-between border-b border-white/10 px-5 py-4">
		<div class="flex flex-col">
			<h2 class="text-lg font-bold text-white">{tagName}</h2>
			{#if articleCount}
				<span class="text-xs text-cyan-400">{articleCount} articles</span>
			{/if}
		</div>
		<button
			type="button"
			onclick={onClose}
			class="rounded-lg p-1.5 text-white/50 transition-colors hover:bg-white/10 hover:text-white"
			aria-label="Close"
		>
			<X class="h-5 w-5" />
		</button>
	</div>

	<!-- Tabs -->
	<div class="flex border-b border-white/10">
		<button
			type="button"
			onclick={() => (activeTab = "articles")}
			class="flex flex-1 items-center justify-center gap-2 py-3 text-sm font-medium transition-colors {activeTab === 'articles'
				? 'border-b-2 border-cyan-400 text-cyan-400'
				: 'text-white/50 hover:text-white/80'}"
		>
			<FileText class="h-4 w-4" />
			Articles
		</button>
		<button
			type="button"
			onclick={() => (activeTab = "recaps")}
			class="flex flex-1 items-center justify-center gap-2 py-3 text-sm font-medium transition-colors {activeTab === 'recaps'
				? 'border-b-2 border-cyan-400 text-cyan-400'
				: 'text-white/50 hover:text-white/80'}"
		>
			<BookOpen class="h-4 w-4" />
			Recaps
		</button>
	</div>

	<!-- Tab Content -->
	<div class="flex-1 overflow-hidden px-4 py-3 flex flex-col">
		{#if activeTab === "articles"}
			<HUDArticlesTab {tagName} />
		{:else}
			<HUDRecapsTab {tagName} />
		{/if}
	</div>
</div>

<style>
	@keyframes slide-in {
		from {
			transform: translateX(100%);
		}
		to {
			transform: translateX(0);
		}
	}

	.animate-slide-in {
		animation: slide-in 0.3s ease-out;
	}
</style>
