<script lang="ts">
import { ArrowUpCircle, FileText, Tag, Info } from "@lucide/svelte";
import type { SupersedeInfoData } from "$lib/connect/knowledge_home";
import { resolveSupersede } from "./supersede-display-map";

interface Props {
	info: SupersedeInfoData;
}

const { info }: Props = $props();

const display = $derived(resolveSupersede(info.state));

const iconMap: Record<string, typeof ArrowUpCircle> = {
	FileText,
	Tag,
	Info,
	ArrowUpCircle,
};
const Icon = $derived(iconMap[display.iconName] ?? ArrowUpCircle);
</script>

<span
	class={`inline-flex items-center gap-1 px-1.5 py-0.5 text-xs rounded border ${display.colorClass}`}
	title={`Updated at ${info.supersededAt}`}
>
	<Icon class="h-3 w-3" />
	{display.label}
</span>
