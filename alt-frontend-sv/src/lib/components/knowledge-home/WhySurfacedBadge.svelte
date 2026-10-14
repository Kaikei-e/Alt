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
import type { WhyReasonData } from "#lib/connect/knowledge_home.js";
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
	class="badge {display.colorClass}"
>
	<IconComponent class="h-3 w-3" />
	{display.label}
</span>

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
</style>
