<script lang="ts">
import { ExternalLink, BirdIcon, Headphones, X } from "@lucide/svelte";

interface Props {
	itemKey: string;
	itemType: string;
	articleId?: string;
	onAction: (type: string) => void;
}

const { onAction }: Props = $props();

const actions = [
	{ type: "open", icon: ExternalLink, label: "Open" },
	{ type: "ask", icon: BirdIcon, label: "Ask Augur" },
	{ type: "listen", icon: Headphones, label: "Listen" },
	{ type: "dismiss", icon: X, label: "Dismiss" },
] as const;
</script>

<div class="flex items-center gap-1">
	{#each actions as action}
		<button
			type="button"
			onclick={() => onAction(action.type)}
			class="p-1.5 rounded-md text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--surface-hover)] transition-colors duration-150"
			title={action.label}
			aria-label={action.label}
		>
			<action.icon class="h-4 w-4" />
		</button>
	{/each}
</div>
