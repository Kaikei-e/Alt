<script lang="ts">
import type { TopicRole } from "$lib/schema/evening_pulse";

interface Props {
	role: TopicRole;
}

const { role }: Props = $props();

const roleConfig = $derived.by(() => {
	switch (role) {
		case "need_to_know":
			return {
				label: "Today's Key",
				bgColor: "var(--pulse-need-to-know-bg, hsl(0 84% 60% / 0.15))",
				textColor: "var(--pulse-need-to-know-text, hsl(0 84% 60%))",
				icon: "!",
			};
		case "trend":
			return {
				label: "Trending",
				bgColor: "var(--pulse-trend-bg, hsl(45 93% 47% / 0.15))",
				textColor: "var(--pulse-trend-text, hsl(45 93% 47%))",
				icon: "\u2191",
			};
		case "serendipity":
			return {
				label: "Discovery",
				bgColor: "var(--pulse-serendipity-bg, hsl(262 83% 58% / 0.15))",
				textColor: "var(--pulse-serendipity-text, hsl(262 83% 58%))",
				icon: "\u2605",
			};
		default:
			return {
				label: "Topic",
				bgColor: "var(--surface-bg)",
				textColor: "var(--text-secondary)",
				icon: "",
			};
	}
});
</script>

<span
	class="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium"
	style="background: {roleConfig.bgColor}; color: {roleConfig.textColor};"
>
	{#if roleConfig.icon}
		<span class="text-[10px]">{roleConfig.icon}</span>
	{/if}
	{roleConfig.label}
</span>
