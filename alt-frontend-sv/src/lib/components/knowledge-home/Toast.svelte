<script lang="ts">
import type { ToastItem } from "$lib/stores/toast.svelte";

interface Props {
	items: ToastItem[];
	onDismiss: (id: string) => void;
}

const { items, onDismiss }: Props = $props();

// Editorial kickers — newspaper metaphor stays in the visual layer only.
const kickerLabel: Record<ToastItem["kind"], string> = {
	success: "DISPATCH",
	error: "DESK // ERROR",
	info: "NOTICE",
};
</script>

{#if items.length > 0}
	<div class="toast-stack" aria-label="Notifications">
		{#each items as item (item.id)}
			<article
				class="toast toast-kind-{item.kind}"
				role={item.kind === "error" ? "alert" : "status"}
				aria-live={item.kind === "error" ? "assertive" : "polite"}
			>
				<header class="toast-kicker">
					<span class="kicker-label">{kickerLabel[item.kind]}</span>
					<span class="kicker-rule" aria-hidden="true"></span>
					<button
						type="button"
						class="toast-dismiss"
						onclick={() => onDismiss(item.id)}
						aria-label="Dismiss notification"
					>×</button>
				</header>
				<p class="toast-message">{item.message}</p>
			</article>
		{/each}
	</div>
{/if}

<style>
	.toast-stack {
		pointer-events: none;
		position: fixed;
		bottom: 1.25rem;
		right: 1.25rem;
		z-index: 50;
		display: flex;
		flex-direction: column;
		gap: 0.625rem;
		max-width: min(22rem, calc(100vw - 2.5rem));
	}

	.toast {
		pointer-events: auto;
		position: relative;
		padding: 0.625rem 0.875rem 0.75rem 1.125rem;
		background: var(--surface-bg, #faf8f3);
		border: 1px solid var(--surface-border, #e5dfce);
		box-shadow:
			0 1px 0 0 var(--surface-border, #e5dfce),
			0 8px 24px -12px rgba(26, 26, 26, 0.18);
		animation: toast-slide 0.32s cubic-bezier(0.2, 0.8, 0.2, 1);
	}

	/* Left-edge column gutter rule — the one newspaper motif we keep per toast. */
	.toast::before {
		content: "";
		position: absolute;
		inset: 0.5rem auto 0.5rem 0;
		width: 2px;
		background: var(--alt-charcoal, #1a1a1a);
	}

	.toast-kind-success::before { background: var(--alt-success, #3f6e3a); }
	.toast-kind-error::before { background: var(--alt-error, #b85450); }

	.toast-kicker {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin-bottom: 0.375rem;
	}

	.kicker-label {
		font-family: var(--font-mono, "IBM Plex Mono", "JetBrains Mono", ui-monospace, monospace);
		font-size: 0.625rem;
		letter-spacing: 0.14em;
		text-transform: uppercase;
		color: var(--alt-ash, #7a7568);
		flex-shrink: 0;
	}

	.toast-kind-success .kicker-label { color: var(--alt-success, #3f6e3a); }
	.toast-kind-error .kicker-label { color: var(--alt-error, #b85450); }

	.kicker-rule {
		flex: 1 1 auto;
		height: 1px;
		background: var(--surface-border, #e5dfce);
	}

	.toast-dismiss {
		flex: 0 0 auto;
		appearance: none;
		background: transparent;
		border: 0;
		padding: 0 0.25rem;
		margin: -0.25rem -0.25rem -0.25rem 0;
		font-family: var(--font-body, "Source Serif 4", "Source Serif Pro", Georgia, serif);
		font-size: 1rem;
		line-height: 1;
		color: var(--alt-ash, #7a7568);
		cursor: pointer;
		transition: color 0.12s ease;
	}

	.toast-dismiss:hover { color: var(--alt-charcoal, #1a1a1a); }
	.toast-dismiss:focus-visible {
		outline: 1px solid var(--alt-charcoal, #1a1a1a);
		outline-offset: 2px;
	}

	.toast-message {
		margin: 0;
		font-family: var(--font-body, "Source Serif 4", "Source Serif Pro", Georgia, serif);
		font-size: 0.9375rem;
		line-height: 1.45;
		color: var(--alt-charcoal, #1a1a1a);
	}

	@keyframes toast-slide {
		from { opacity: 0; transform: translateX(16px); }
		to   { opacity: 1; transform: translateX(0); }
	}

	@media (prefers-reduced-motion: reduce) {
		.toast { animation: none; }
	}
</style>
