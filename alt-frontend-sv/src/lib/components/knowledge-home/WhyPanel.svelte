<script lang="ts">
import { Info } from "@lucide/svelte";
import type { WhyReasonData } from "$lib/connect/knowledge_home";
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

const categoryLabel: Record<string, string> = {
	source: "Source",
	behavior: "Behavior",
	semantic: "Semantic",
	change: "Change",
	other: "Other",
};

const categorized = $derived.by(() => {
	const groups: { key: string; label: string; items: WhyReasonData[] }[] = [];
	const bucket = new Map<string, WhyReasonData[]>();

	for (const reason of reasons) {
		const cat = categorize(reason.code);
		const items = bucket.get(cat) ?? [];
		items.push(reason);
		bucket.set(cat, items);
	}

	for (const [key, items] of bucket.entries()) {
		groups.push({ key, label: categoryLabel[key] ?? "Other", items });
	}

	return groups;
});
</script>

<div class="animate-fade-up  border border-[var(--surface-border)] bg-[var(--surface-bg)] p-3">
	<h4 class="text-xs font-semibold text-[var(--text-secondary)] uppercase tracking-wider mb-2 flex items-center gap-1.5">
		<Info class="h-3.5 w-3.5" />
		Why this was surfaced
	</h4>

	{#if hasReasons}
		<div class="space-y-3">
			{#each categorized as group}
				<div class="space-y-1.5">
					<p class="text-xs font-semibold uppercase tracking-wider text-[var(--text-secondary)]">
						{group.label}
					</p>
					<div class="space-y-1.5">
						{#each group.items as reason}
							<div class=" border border-[var(--surface-border)] bg-[var(--surface-hover)] px-3 py-2.5">
								<p class="text-xs font-medium text-[var(--text-primary)]">
									{resolveWhyReason(reason.code, reason.tag).label}
								</p>
								<p class="mt-0.5 text-xs leading-relaxed text-[var(--text-secondary)]">
									{#if reason.code === "new_unread"}
										Surfaced as a new, unread candidate.
									{:else if reason.code === "in_weekly_recap"}
										Connected to a topic covered in the weekly Recap.
									{:else if reason.code === "tag_hotspot" && reason.tag}
										Related to the trending tag "{reason.tag}".
									{:else if reason.code === "pulse_need_to_know"}
										Prioritized as a need-to-know candidate for today.
									{:else if reason.code === "recent_interest_match"}
										Selected based on themes similar to your recent activity.
									{:else if reason.code === "related_to_recent_search"}
										Related to your recent search context.
									{:else if reason.code === "summary_completed"}
										Summary generation completed — key points are now available.
									{:else}
										Displayed based on Home's relevance assessment.
									{/if}
								</p>
							</div>
						{/each}
					</div>
				</div>
			{/each}
		</div>
	{:else}
		<p class="text-xs text-[var(--text-tertiary)]">
			Matched by general relevance
		</p>
	{/if}
</div>
