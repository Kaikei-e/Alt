<script lang="ts">
import type { TopicRole } from "#lib/schema/evening_pulse.js";

interface Props {
	role: TopicRole;
}

const { role }: Props = $props();

const roleConfig = $derived.by(() => {
	switch (role) {
		case "need_to_know":
			return {
				label: "Need to know",
				bgColor: "var(--accent-emphasis-bg)",
				textColor: "var(--accent-emphasis-text)",
				borderColor: "var(--accent-emphasis-border)",
			};
		case "trend":
			return {
				label: "Trending",
				bgColor: "var(--accent-muted-bg)",
				textColor: "var(--accent-muted-text)",
				borderColor: "var(--accent-muted-border)",
			};
		case "serendipity":
			return {
				label: "Discovery",
				bgColor: "var(--accent-info-bg)",
				textColor: "var(--accent-info-text)",
				borderColor: "var(--accent-info-border)",
			};
		default:
			return {
				label: "Topic",
				bgColor: "var(--surface-2)",
				textColor: "var(--text-secondary)",
				borderColor: "var(--surface-border)",
			};
	}
});
</script>

<span
	class="inline-flex items-center px-2 py-0.5 text-[11px] font-semibold uppercase tracking-wider border font-mono"
	style="background: {roleConfig.bgColor}; color: {roleConfig.textColor}; border-color: {roleConfig.borderColor};"
>
	{roleConfig.label}
</span>
