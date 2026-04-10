<script lang="ts">
import { BirdIcon, ExternalLink, Headphones, X } from "@lucide/svelte";

interface Props {
	itemKey: string;
	itemType: string;
	articleId?: string;
	onAction: (type: string) => void;
}

const { onAction }: Props = $props();

const primaryActions = $derived([
	{ type: "open", icon: ExternalLink, label: "Open" },
	{ type: "ask", icon: BirdIcon, label: "Ask" },
	{ type: "listen", icon: Headphones, label: "Listen" },
]);
</script>

<div class="action-group">
	{#each primaryActions as action}
		<button
			type="button"
			onclick={() => onAction(action.type)}
			class="action-btn"
			title={action.label}
			aria-label={action.label}
		>
			<action.icon class="h-4 w-4" />
			<span class="hidden md:inline">{action.label}</span>
		</button>
	{/each}
	<button
		type="button"
		onclick={() => onAction("dismiss")}
		class="action-btn action-btn--dismiss"
		title="Dismiss"
		aria-label="Dismiss"
	>
		<X class="h-4 w-4" />
	</button>
</div>

<style>
	.action-group {
		display: flex;
		flex-shrink: 0;
		align-items: center;
		gap: 0.125rem;
		border: 1px solid var(--chip-border);
		background: var(--action-surface);
		padding: 0.125rem 0.25rem;
	}

	.action-btn {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
		padding: 0.375rem 0.5rem;
		font-size: 0.75rem;
		font-weight: 500;
		color: var(--interactive-text);
		background: transparent;
		border: none;
		cursor: pointer;
		transition: background 0.15s, color 0.15s;
	}

	.action-btn:hover {
		background: var(--action-surface-hover);
		color: var(--interactive-text-hover);
	}

	.action-btn--dismiss {
		margin-left: 0.5rem;
		border-left: 1px solid color-mix(in srgb, var(--surface-border) 50%, transparent);
		padding-left: 0.5rem;
		color: color-mix(in srgb, var(--alt-slate) 70%, transparent);
	}

	.action-btn--dismiss:hover {
		background: var(--action-surface-hover);
		color: var(--alt-charcoal);
	}
</style>
