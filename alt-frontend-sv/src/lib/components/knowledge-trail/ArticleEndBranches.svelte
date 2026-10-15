<script lang="ts">
import { onMount } from "svelte";
import type {
	BranchData,
	ResolveBranchHandler,
} from "#lib/connect/knowledge_trail.js";
import { useArticleEndBranches } from "#lib/hooks/useArticleEndBranches.svelte.js";
import BranchCard from "./BranchCard.svelte";

interface Props {
	branches?: BranchData[];
	onResolve?: ResolveBranchHandler;
	anchorItemKey?: string;
}

const { branches, onResolve, anchorItemKey }: Props = $props();

const ownBranches = useArticleEndBranches();

onMount(() => {
	if (anchorItemKey) {
		void ownBranches.load(anchorItemKey);
	}
});

const effectiveBranches = $derived(branches ?? ownBranches.branches);
const effectiveResolve = $derived<ResolveBranchHandler>(
	onResolve ?? ownBranches.resolve,
);

// The article read-end is the branch's main stage (D26) — capped at 2 so the
// proposal never competes with the article itself for attention.
const visible = $derived(effectiveBranches.slice(0, 2));
</script>

{#if visible.length > 0}
	<section
		class="article-end-branches"
		data-testid="article-end-branches"
		aria-label="Where this leads"
	>
		<h2 class="section-heading">Where this leads</h2>
		{#each visible as branch (branch.branchKey)}
			<BranchCard {branch} testId="article-end-branch" onResolve={effectiveResolve} />
		{/each}
	</section>
{/if}

<style>
	.article-end-branches {
		margin-top: 2rem;
		padding-top: 1.25rem;
		border-top: 1px solid var(--surface-border, #c8c8c8);
	}
	.section-heading {
		font-family: var(--font-display);
		font-size: 1.05rem;
		font-weight: 700;
		color: var(--alt-charcoal, #1a1a1a);
		margin: 0 0 0.75rem;
	}
</style>
