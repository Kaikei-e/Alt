<script lang="ts">
import {
	ExternalLink,
	BirdIcon,
	Headphones,
	Sparkles,
	X,
} from "@lucide/svelte";

interface Props {
	itemKey: string;
	itemType: string;
	articleId?: string;
	onAction: (type: string) => void;
}

const { onAction }: Props = $props();

const primaryActions = [
	{ type: "open", icon: ExternalLink, label: "Open" },
	{ type: "summarize", icon: Sparkles, label: "Summarize" },
	{ type: "ask", icon: BirdIcon, label: "Ask" },
	{ type: "listen", icon: Headphones, label: "Listen" },
] as const;
</script>

<div
	class="flex flex-shrink-0 items-center gap-0.5 rounded-md border border-[var(--chip-border)] bg-[var(--action-surface)] px-1 py-0.5"
>
	{#each primaryActions as action}
		<button
			type="button"
			onclick={() => onAction(action.type)}
			class="inline-flex items-center gap-1 rounded-md px-1.5 py-1 text-xs font-medium text-[var(--interactive-text)] hover:bg-[var(--action-surface-hover)] hover:text-[var(--interactive-text-hover)] transition-colors duration-150"
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
		class="ml-1 rounded-md p-1 text-[var(--text-secondary)]/70 hover:bg-[var(--action-surface-hover)] hover:text-[var(--text-primary)] transition-colors duration-150"
		title="Dismiss"
		aria-label="Dismiss"
	>
		<X class="h-3.5 w-3.5" />
	</button>
</div>
