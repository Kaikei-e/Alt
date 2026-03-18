<script lang="ts">
import {
	ArrowUpCircle,
	FileText,
	Tag,
	Info,
	ChevronDown,
	ChevronUp,
} from "@lucide/svelte";
import type { SupersedeInfoData } from "$lib/connect/knowledge_home";
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
	class={`inline-flex items-center gap-1 px-1.5 py-0.5 text-xs rounded border cursor-pointer transition-colors ${display.colorClass}`}
	title={`Updated at ${info.supersededAt}`}
	onclick={onToggle}
>
	<Icon class="h-3 w-3" />
	{display.label}
	{#if onToggle}
		<Chevron class="h-3 w-3 opacity-60" />
	{/if}
</button>
