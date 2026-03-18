<script lang="ts">
import { ExternalLink, BirdIcon, Headphones, X } from "@lucide/svelte";

interface Props {
	itemKey: string;
	itemType: string;
	articleId?: string;
	onAction: (type: string) => void;
}

const { onAction }: Props = $props();

const primaryActions = [
	{ type: "open", icon: ExternalLink, label: "Open" },
	{ type: "ask", icon: BirdIcon, label: "Ask" },
	{ type: "listen", icon: Headphones, label: "Listen" },
] as const;
</script>

<div class="flex items-center gap-0.5 flex-shrink-0">
	{#each primaryActions as action}
		<button
			type="button"
			onclick={() => onAction(action.type)}
			class="inline-flex items-center gap-1 px-1.5 py-1 rounded-md text-xs text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--surface-hover)] transition-colors duration-150"
			title={action.label}
			aria-label={action.label}
		>
			<action.icon class="h-3.5 w-3.5" />
			<span class="hidden md:inline">{action.label}</span>
		</button>
	{/each}
	<button
		type="button"
		onclick={() => onAction("dismiss")}
		class="p-1 rounded-md text-[var(--text-secondary)]/50 hover:text-[var(--text-primary)] hover:bg-[var(--surface-hover)] transition-colors duration-150 ml-1"
		title="Dismiss"
		aria-label="Dismiss"
	>
		<X class="h-3.5 w-3.5" />
	</button>
</div>
