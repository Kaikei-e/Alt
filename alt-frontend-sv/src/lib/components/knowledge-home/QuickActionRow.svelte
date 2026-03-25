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

<div
	class="flex flex-shrink-0 items-center gap-0.5 rounded-md border border-[var(--chip-border)] bg-[var(--action-surface)] px-1 py-0.5"
>
	{#each primaryActions as action}
		<button
			type="button"
			onclick={() => onAction(action.type)}
			class="inline-flex items-center gap-1 rounded-md px-2 py-1.5 text-xs font-medium text-[var(--interactive-text)] hover:bg-[var(--action-surface-hover)] hover:text-[var(--interactive-text-hover)] transition-colors duration-150"
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
		class="ml-2 border-l border-[var(--surface-border)]/50 pl-2 rounded-md p-1.5 text-[var(--text-secondary)]/70 hover:bg-[var(--action-surface-hover)] hover:text-[var(--text-primary)] transition-colors duration-150"
		title="Dismiss"
		aria-label="Dismiss"
	>
		<X class="h-4 w-4" />
	</button>
</div>
