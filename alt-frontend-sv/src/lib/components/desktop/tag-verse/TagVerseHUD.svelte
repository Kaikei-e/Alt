<script lang="ts">
import { X } from "@lucide/svelte";
import { onMount } from "svelte";
import HUDArticlesTab from "./HUDArticlesTab.svelte";
import HUDRecapsTab from "./HUDRecapsTab.svelte";
import { formatCountLabel } from "./tagIndex.js";

interface Props {
	tagName: string;
	articleCount?: number;
	reducedMotion?: boolean;
	onClose: () => void;
}

let {
	tagName,
	articleCount = 0,
	reducedMotion = false,
	onClose,
}: Props = $props();

const uid = $props.id();
const headingId = `${uid}-detail-heading`;
const articlesTabId = `${uid}-tab-articles`;
const recapsTabId = `${uid}-tab-recaps`;
const articlesPanelId = `${uid}-panel-articles`;
const recapsPanelId = `${uid}-panel-recaps`;

let activeTab = $state<"articles" | "recaps">("articles");
let recapsActivated = $state(false);
let panelEl = $state<HTMLElement | null>(null);
let headingEl = $state<HTMLHeadingElement | null>(null);
let articlesTabEl = $state<HTMLButtonElement | null>(null);
let recapsTabEl = $state<HTMLButtonElement | null>(null);

let previouslyFocused: HTMLElement | null = null;
let focusRestored = false;
let lastFocusedTag: string | null = null;

function isElementFocusableAndVisible(
	el: HTMLElement | null,
): el is HTMLElement {
	if (!el || !document.contains(el)) return false;
	if (el.closest("[hidden]")) return false;
	const style = window.getComputedStyle(el);
	return style.display !== "none" && style.visibility !== "hidden";
}

function restoreFocus() {
	if (focusRestored) return;
	focusRestored = true;

	if (isElementFocusableAndVisible(previouslyFocused)) {
		previouslyFocused.focus();
		return;
	}

	const searchInput = document.querySelector<HTMLElement>(
		'input[type="search"]',
	);
	if (isElementFocusableAndVisible(searchInput)) {
		searchInput.focus();
		return;
	}

	const mainHeading = document.querySelector<HTMLElement>("h1");
	if (isElementFocusableAndVisible(mainHeading)) {
		mainHeading.focus();
	}
}

function closePanel() {
	restoreFocus();
	onClose();
}

$effect(() => {
	const currentTag = tagName;
	if (!headingEl || currentTag === lastFocusedTag) return;
	lastFocusedTag = currentTag;
	focusRestored = false;

	if (
		document.activeElement instanceof HTMLElement &&
		(!panelEl || !panelEl.contains(document.activeElement))
	) {
		previouslyFocused = document.activeElement;
	}
	headingEl.focus();
});

onMount(() => {
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

function isEditableTarget(target: EventTarget | null): boolean {
	if (!(target instanceof HTMLElement)) return false;
	const tag = target.tagName;
	if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT") {
		return true;
	}
	return (
		target.isContentEditable ||
		target.getAttribute("contenteditable") === "true"
	);
}

function handleWindowKeydown(e: KeyboardEvent) {
	if (e.key !== "Escape") return;
	if (e.defaultPrevented || e.isComposing || e.keyCode === 229) return;
	if (
		isEditableTarget(e.target) &&
		(!panelEl || !(e.target instanceof Node) || !panelEl.contains(e.target))
	) {
		return;
	}
	closePanel();
}
</script>

<svelte:window onkeydown={handleWindowKeydown} />

<aside
	bind:this={panelEl}
	aria-labelledby={headingId}
	class="
		absolute right-0 top-0 bottom-0 z-20 flex h-full w-[400px] shrink-0 flex-col border-l border-[var(--surface-border)] bg-[var(--surface-bg)] text-[var(--text-primary)] {reducedMotion
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
				id={headingId}
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
				{formatCountLabel(articleCount, "article")}
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
			id={articlesTabId}
			type="button"
			role="tab"
			aria-selected={activeTab === "articles"}
			aria-controls={articlesPanelId}
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
			id={recapsTabId}
			type="button"
			role="tab"
			aria-selected={activeTab === "recaps"}
			aria-controls={recapsPanelId}
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
		id={articlesPanelId}
		role="tabpanel"
		aria-labelledby={articlesTabId}
		hidden={activeTab !== "articles"}
		class="flex min-h-0 flex-1 flex-col overflow-hidden"
	>
		<HUDArticlesTab {tagName} />
	</div>

	<div
		id={recapsPanelId}
		role="tabpanel"
		aria-labelledby={recapsTabId}
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
</style>
