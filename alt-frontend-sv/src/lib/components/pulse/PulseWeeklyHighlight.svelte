<script lang="ts">
import type { WeeklyHighlight } from "#lib/schema/evening_pulse.js";

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
			return "var(--accent-emphasis-text)";
		case "trend":
			return "var(--accent-muted-text)";
		case "serendipity":
			return "var(--accent-info-text)";
		default:
			return "var(--text-secondary)";
	}
});
</script>

<button
	type="button"
	class="w-full text-left p-4 border transition-colors cursor-pointer"
	style="background: var(--surface-bg); border-color: var(--surface-border); border-left: 3px solid {roleColor};"
	{onclick}
	aria-label="View highlight: {highlight.title}"
>
	<h4
		class="text-sm font-semibold mb-1 line-clamp-2"
		style="color: var(--text-primary); font-family: var(--font-display);"
	>
		{highlight.title}
	</h4>
	<p class="text-xs font-mono" style="color: var(--text-muted);">
		{formattedDate}
	</p>
</button>
