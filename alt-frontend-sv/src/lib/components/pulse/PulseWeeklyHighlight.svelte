<script lang="ts">
import type { WeeklyHighlight } from "$lib/schema/evening_pulse";

interface Props {
	highlight: WeeklyHighlight;
	onclick?: () => void;
}

const { highlight, onclick }: Props = $props();

const formattedDate = $derived.by(() => {
	try {
		const d = new Date(highlight.date);
		return d.toLocaleDateString("en-US", {
			month: "short",
			day: "numeric",
		});
	} catch {
		return highlight.date;
	}
});

const roleColor = $derived.by(() => {
	switch (highlight.role) {
		case "need_to_know":
			return "var(--pulse-need-to-know-text, hsl(0 84% 60%))";
		case "trend":
			return "var(--pulse-trend-text, hsl(45 93% 47%))";
		case "serendipity":
			return "var(--pulse-serendipity-text, hsl(262 83% 58%))";
		default:
			return "var(--text-secondary)";
	}
});
</script>

<button
	type="button"
	class="w-full text-left p-4 border-2 border-[var(--surface-border)] bg-white transition-all duration-200 hover:shadow-md hover:-translate-y-0.5"
	style="border-left: 4px solid {roleColor};"
	{onclick}
	aria-label="View highlight: {highlight.title}"
>
	<h4
		class="text-sm font-semibold mb-1 line-clamp-2"
		style="color: var(--text-primary);"
	>
		{highlight.title}
	</h4>
	<p class="text-xs" style="color: var(--text-muted);">
		{formattedDate}
	</p>
</button>
