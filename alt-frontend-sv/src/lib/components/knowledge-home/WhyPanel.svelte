<script lang="ts">
import { Info } from "@lucide/svelte";
import type { WhyReasonData } from "$lib/connect/knowledge_home";
import WhySurfacedBadge from "./WhySurfacedBadge.svelte";
import { resolveWhyReason } from "./why-reason-map";

interface Props {
	reasons: WhyReasonData[];
}

const { reasons }: Props = $props();

const hasReasons = $derived(reasons.length > 0);

/**
 * Categorize why reasons for structured display.
 * Categories from the plan: source_why, behavior_why, semantic_why, change_why
 */
const SOURCE_CODES = new Set([
	"new_unread",
	"in_weekly_recap",
	"pulse_need_to_know",
]);
const BEHAVIOR_CODES = new Set([
	"recent_interest_match",
	"related_to_recent_search",
]);
const SEMANTIC_CODES = new Set(["tag_hotspot"]);
const CHANGE_CODES = new Set(["summary_completed"]);

function categorize(code: string): string {
	if (SOURCE_CODES.has(code)) return "source";
	if (BEHAVIOR_CODES.has(code)) return "behavior";
	if (SEMANTIC_CODES.has(code)) return "semantic";
	if (CHANGE_CODES.has(code)) return "change";
	return "other";
}

const categorized = $derived.by(() => {
	const groups: Record<string, WhyReasonData[]> = {};
	for (const reason of reasons) {
		const cat = categorize(reason.code);
		if (!groups[cat]) groups[cat] = [];
		groups[cat].push(reason);
	}
	return groups;
});
</script>

<div class="rounded-lg border border-[var(--surface-border)] bg-[var(--surface-bg)] p-3">
	<h4 class="text-xs font-semibold text-[var(--text-secondary)] uppercase tracking-wider mb-2 flex items-center gap-1.5">
		<Info class="h-3.5 w-3.5" />
		Why this was surfaced
	</h4>

	{#if hasReasons}
		<div class="flex flex-wrap gap-1.5">
			{#each reasons as reason}
				<WhySurfacedBadge {reason} />
			{/each}
		</div>
	{:else}
		<p class="text-xs text-[var(--text-tertiary)]">
			Matched by general relevance
		</p>
	{/if}
</div>
