<script lang="ts">
import { BirdIcon, ExternalLink, X } from "@lucide/svelte";

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
		gap: 0.15rem;
	}

	.action-btn {
		display: inline-flex;
		align-items: center;
		gap: 0.35rem;
		padding: 0.35rem 0.55rem;
		font-family: var(--font-body);
		font-size: 0.8rem;
		font-weight: 500;
		color: var(--interactive-text);
		background: transparent;
		border: 1px solid transparent;
		cursor: pointer;
		transition: background 0.15s, border-color 0.15s, color 0.15s;
	}

	.action-btn:hover {
		background: var(--action-surface);
		border-color: var(--chip-border);
		color: var(--interactive-text-hover);
	}

	.action-btn--dismiss {
		margin-left: 0.35rem;
		color: var(--alt-ash);
	}

	.action-btn--dismiss:hover {
		background: var(--surface-hover);
		border-color: var(--surface-border);
		color: var(--alt-charcoal);
	}
</style>
