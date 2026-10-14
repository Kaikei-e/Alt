<script lang="ts">
import {
	ArrowUpCircle,
	ChevronDown,
	ChevronUp,
	FileText,
	Info,
	Tag,
} from "@lucide/svelte";
import type { SupersedeInfoData } from "#lib/connect/knowledge_home.js";
import { resolveSupersede } from "./supersede-display-map";

interface Props {
	info: SupersedeInfoData;
	expanded?: boolean;
	onToggle?: () => void;
}

const { info, expanded = false, onToggle }: Props = $props();

const display = $derived(resolveSupersede(info.state));

const iconMap: Record<string, typeof ArrowUpCircle> = {
	FileText,
	Tag,
	Info,
	ArrowUpCircle,
};
const Icon = $derived(iconMap[display.iconName] ?? ArrowUpCircle);
const Chevron = $derived(expanded ? ChevronUp : ChevronDown);
</script>

<button
	type="button"
	class={`badge badge--toggle ${display.colorClass}`}
	title={`Updated at ${info.supersededAt}`}
	onclick={onToggle}
>
	<Icon class="h-3 w-3" />
	{display.label}
	{#if onToggle}
		<Chevron class="h-3 w-3 opacity-60" />
	{/if}
</button>

<style>
	.badge {
		display: inline-flex;
		align-items: center;
		gap: 0.3rem;
		padding: 0.14rem 0.5rem;
		font-family: var(--font-mono);
		font-size: 0.62rem;
		font-weight: 600;
		letter-spacing: 0.1em;
		line-height: 1.4;
		text-transform: uppercase;
		border-width: 1px;
		border-style: solid;
		white-space: nowrap;
	}

	.badge--toggle {
		cursor: pointer;
		transition: border-color 0.15s;
	}

	.badge--toggle:hover {
		border-color: currentColor;
	}
</style>
