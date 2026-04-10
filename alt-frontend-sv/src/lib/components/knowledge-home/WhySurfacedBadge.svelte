<script lang="ts">
import {
	Activity,
	CalendarRange,
	FileText,
	Info,
	Search,
	Sparkles,
	Star,
	Tag,
} from "@lucide/svelte";
import type { WhyReasonData } from "$lib/connect/knowledge_home";
import { resolveWhyReason } from "./why-reason-map";

const ICON_MAP: Record<string, typeof Sparkles> = {
	Sparkles,
	CalendarRange,
	Tag,
	FileText,
	Activity,
	Star,
	Search,
	Info,
};

interface Props {
	reason: WhyReasonData;
}

const { reason }: Props = $props();

const display = $derived(resolveWhyReason(reason.code, reason.tag));
const IconComponent = $derived(ICON_MAP[display.iconName] ?? Info);
</script>

<span
	class="inline-flex items-center gap-1 px-2 py-0.5 text-xs font-medium border {display.colorClass}"
>
	<IconComponent class="h-3 w-3" />
	{display.label}
</span>
