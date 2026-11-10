<script lang="ts">
import { X } from "@lucide/svelte";
import { onMount } from "svelte";
import { MediaQuery } from "svelte/reactivity";
import HUDArticlesTab from "./HUDArticlesTab.svelte";
import HUDRecapsTab from "./HUDRecapsTab.svelte";

interface Props {
	tagName: string;
	articleCount?: number;
	reducedMotion?: boolean;
	onClose: () => void;
}

let {
	tagName,
	articleCount = 0,
	reducedMotion: reducedMotionProp,
	onClose,
}: Props = $props();

const reducedMotionQuery = new MediaQuery(
	"prefers-reduced-motion: reduce",
	false,
);
const isReducedMotion = $derived(
	reducedMotionProp ?? reducedMotionQuery.current,
);

let activeTab = $state<"articles" | "recaps">("articles");
let recapsActivated = $state(false);
let headingEl = $state<HTMLHeadingElement | null>(null);
let articlesTabEl = $state<HTMLButtonElement | null>(null);
let recapsTabEl = $state<HTMLButtonElement | null>(null);

let previouslyFocused: HTMLElement | null = null;
let focusRestored = false;

function restoreFocus() {
	if (
		!focusRestored &&
		previouslyFocused &&
		document.contains(previouslyFocused)
	) {
		focusRestored = true;
		previouslyFocused.focus();
	}
}

function closePanel() {
	restoreFocus();
	onClose();
}

onMount(() => {
	previouslyFocused =
		document.activeElement instanceof HTMLElement
			? document.activeElement
			: null;
	headingEl?.focus();

	return () => {
		restoreFocus();
	};
});

function selectTab(tab: "articles" | "recaps") {
	activeTab = tab;
	if (tab === "recaps") {
		recapsActivated = true;
	}
}

function handleTabKeydown(e: KeyboardEvent, current: "articles" | "recaps") {
	if (e.key === "ArrowRight" || e.key === "ArrowLeft") {
		e.preventDefault();
		const next = current === "articles" ? "recaps" : "articles";
		selectTab(next);
		if (next === "articles") {
			articlesTabEl?.focus();
		} else {
			recapsTabEl?.focus();
		}
	} else if (e.key === "Home") {
		e.preventDefault();
		selectTab("articles");
		articlesTabEl?.focus();
	} else if (e.key === "End") {
		e.preventDefault();
		selectTab("recaps");
		recapsTabEl?.focus();
	}
}

function handleWindowKeydown(e: KeyboardEvent) {
	if (e.key === "Escape" && !e.defaultPrevented) {
		closePanel();
	}
}
</script>

<svelte:window onkeydown={handleWindowKeydown} />

<aside
	aria-labelledby="tag-verse-detail-heading"
	class="flex h-full w-[400px] shrink-0 flex-col border-l border-[var(--surface-border)] bg-[var(--surface-bg)] text-[var(--text-primary)] {isReducedMotion
		? ''
		: 'animate-slide-in-panel'}"
>
	<div
		class="flex items-start justify-between border-b border-[var(--surface-border)] px-5 py-4"
	>
		<div class="flex flex-col gap-0.5">
			<span
				class="text-[11px] uppercase tracking-wider text-[var(--text-muted)]"
				style="font-family: var(--font-mono);"
			>
				Tag
			</span>
			<h2
				id="tag-verse-detail-heading"
				bind:this={headingEl}
				tabindex="-1"
				class="text-lg font-bold text-[var(--text-primary)] focus:outline-none"
				style="font-family: var(--font-display);"
			>
				{tagName}
			</h2>
			<span
				class="text-xs text-[var(--text-muted)]"
				style="font-family: var(--font-mono);"
			>
				{articleCount} articles
			</span>
		</div>
		<button
			type="button"
			onclick={closePanel}
			aria-label="Close tag details"
			class="rounded-none border border-[var(--surface-border)] bg-[var(--action-surface)] p-1.5 text-[var(--text-primary)] transition-colors hover:bg-[var(--action-surface-hover)]"
		>
			<X class="h-4 w-4" />
		</button>
	</div>

	<div
		role="tablist"
		aria-label="Tag details"
		class="flex border-b border-[var(--surface-border)]"
	>
		<button
			bind:this={articlesTabEl}
			id="tag-verse-tab-articles"
			type="button"
			role="tab"
			aria-selected={activeTab === "articles"}
			aria-controls="tag-verse-panel-articles"
			tabindex={activeTab === "articles" ? 0 : -1}
			onclick={() => selectTab("articles")}
			onkeydown={(e) => handleTabKeydown(e, "articles")}
			class="flex flex-1 items-center justify-center py-2.5 text-sm font-medium transition-colors {activeTab ===
			'articles'
				? 'border-b-2 border-[var(--text-primary)] text-[var(--text-primary)]'
				: 'border-b-2 border-transparent text-[var(--text-muted)] hover:text-[var(--text-primary)]'}"
		>
			Articles
		</button>
		<button
			bind:this={recapsTabEl}
			id="tag-verse-tab-recaps"
			type="button"
			role="tab"
			aria-selected={activeTab === "recaps"}
			aria-controls="tag-verse-panel-recaps"
			tabindex={activeTab === "recaps" ? 0 : -1}
			onclick={() => selectTab("recaps")}
			onkeydown={(e) => handleTabKeydown(e, "recaps")}
			class="flex flex-1 items-center justify-center py-2.5 text-sm font-medium transition-colors {activeTab ===
			'recaps'
				? 'border-b-2 border-[var(--text-primary)] text-[var(--text-primary)]'
				: 'border-b-2 border-transparent text-[var(--text-muted)] hover:text-[var(--text-primary)]'}"
		>
			Recaps
		</button>
	</div>

	<div
		id="tag-verse-panel-articles"
		role="tabpanel"
		aria-labelledby="tag-verse-tab-articles"
		hidden={activeTab !== "articles"}
		class="flex min-h-0 flex-1 flex-col overflow-hidden"
	>
		<HUDArticlesTab {tagName} />
	</div>

	<div
		id="tag-verse-panel-recaps"
		role="tabpanel"
		aria-labelledby="tag-verse-tab-recaps"
		hidden={activeTab !== "recaps"}
		class="flex min-h-0 flex-1 flex-col overflow-hidden"
	>
		{#if recapsActivated || activeTab === "recaps"}
			<HUDRecapsTab {tagName} />
		{/if}
	</div>
</aside>

<style>
	@keyframes slide-in-panel {
		from {
			transform: translateX(100%);
		}
		to {
			transform: translateX(0);
		}
	}

	.animate-slide-in-panel {
		animation: slide-in-panel 0.2s ease-out;
	}

	@media (prefers-reduced-motion: reduce) {
		.animate-slide-in-panel {
			animation: none;
		}
	}
</style>
