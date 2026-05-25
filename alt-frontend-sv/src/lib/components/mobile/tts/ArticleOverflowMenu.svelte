<script lang="ts">
import { Headphones, MoreVertical } from "@lucide/svelte";

interface Props {
	onListenSelect: () => void;
}

const { onListenSelect }: Props = $props();

let open = $state(false);
let triggerEl = $state<HTMLButtonElement | null>(null);
let menuEl = $state<HTMLDivElement | null>(null);

function closeAndRefocusTrigger() {
	open = false;
	triggerEl?.focus();
}

function toggle() {
	open = !open;
}

function handleListen() {
	onListenSelect();
	closeAndRefocusTrigger();
}

function handleKeyDown(event: KeyboardEvent) {
	if (event.key === "Escape") {
		event.preventDefault();
		closeAndRefocusTrigger();
	}
}

function handleDocumentPointer(event: MouseEvent) {
	if (!open) return;
	const target = event.target as Node | null;
	if (!target) return;
	if (menuEl?.contains(target)) return;
	if (triggerEl?.contains(target)) return;
	open = false;
}

$effect(() => {
	if (!open) return;
	const handler = handleDocumentPointer;
	document.addEventListener("pointerdown", handler, true);
	return () => document.removeEventListener("pointerdown", handler, true);
});
</script>

<div class="overflow-root">
	<button
		type="button"
		bind:this={triggerEl}
		aria-label="Article actions"
		aria-haspopup="menu"
		aria-expanded={open}
		data-testid="article-overflow-trigger"
		class="overflow-trigger"
		onclick={toggle}
		onkeydown={handleKeyDown}
	>
		<MoreVertical class="h-4 w-4" aria-hidden="true" />
	</button>

	{#if open}
		<div
			bind:this={menuEl}
			role="menu"
			tabindex="-1"
			aria-label="Article actions"
			class="overflow-menu"
			onkeydown={handleKeyDown}
		>
			<button
				type="button"
				role="menuitem"
				class="overflow-menu__item"
				data-testid="overflow-listen-item"
				onclick={handleListen}
			>
				<Headphones class="h-4 w-4" aria-hidden="true" />
				<span>Listen to article</span>
			</button>
		</div>
	{/if}
</div>

<style>
	.overflow-root {
		position: relative;
		display: inline-flex;
	}

	.overflow-trigger {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 2.25rem;
		height: 2.25rem;
		background: transparent;
		border: 1px solid var(--surface-border, #c8c8c8);
		color: var(--interactive-text, #2f4f4f);
		cursor: pointer;
		transition: background 0.15s ease;
	}
	.overflow-trigger:hover,
	.overflow-trigger:focus-visible {
		background: var(--surface-hover, #f3f1ed);
		outline: none;
	}

	.overflow-menu {
		position: absolute;
		right: 0;
		top: calc(100% + 0.3rem);
		min-width: 12rem;
		padding: 0.25rem;
		background: var(--surface-bg, #faf9f7);
		border: 1px solid var(--surface-border, #c8c8c8);
		box-shadow: 0 8px 24px rgba(0, 0, 0, 0.08);
		z-index: 60;
	}

	.overflow-menu__item {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		width: 100%;
		padding: 0.6rem 0.75rem;
		background: transparent;
		border: 0;
		font-family: var(--font-body);
		font-size: 0.85rem;
		color: var(--alt-charcoal, #1a1a1a);
		text-align: left;
		cursor: pointer;
	}
	.overflow-menu__item:hover,
	.overflow-menu__item:focus-visible {
		background: var(--surface-hover, #f3f1ed);
		outline: none;
	}
</style>
