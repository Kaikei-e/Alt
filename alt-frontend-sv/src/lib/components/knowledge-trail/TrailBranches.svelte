<script lang="ts">
import type { BranchData } from "$lib/connect/knowledge_trail";

interface Props {
	branches: BranchData[];
}

const { branches }: Props = $props();

const KIND_LABEL: Record<string, string> = {
	continuation: "Continues your thread",
	cluster: "Joins a topic you follow",
	contradiction: "Challenges what you read",
	inquiry: "Answers your question",
};

function kindLabel(kind: string): string {
	return KIND_LABEL[kind] ?? kind;
}

function evidenceSummary(b: BranchData): string {
	return b.evidenceRefs.map((r) => r.label).join(" · ");
}
</script>

{#if branches.length > 0}
	<section class="branches" data-testid="trail-branches">
		<h2 class="branches-heading">Branches</h2>
		{#each branches as branch (branch.branchKey)}
			<article
				class="branch kind-{branch.relationKind}"
				data-testid="trail-branch"
				data-relation-kind={branch.relationKind}
			>
				<span class="branch-kind">{kindLabel(branch.relationKind)}</span>
				<div class="branch-title">{branch.targetTitle || branch.targetItemKey}</div>
				<p class="branch-why">{branch.why}</p>
				<div class="branch-evidence">
					evidence: {evidenceSummary(branch)} &nbsp;·&nbsp; confidence: {branch.confidence}
				</div>
			</article>
		{/each}
	</section>
{/if}

<style>
	.branches {
		margin-top: 1.5rem;
		max-width: 880px;
	}
	.branches-heading {
		font-family: var(--font-display);
		font-size: 1rem;
		font-weight: 700;
		color: var(--alt-charcoal, #1a1a1a);
		margin: 0 0 0.6rem;
	}
	.branch {
		border: 1px solid var(--surface-border, #c8c8c8);
		border-left-width: 3px;
		background: var(--surface-2, #f5f4f1);
		padding: 0.8rem 1rem 0.85rem;
		margin-bottom: 0.7rem;
	}
	.kind-contradiction {
		border-left-color: var(--accent-emphasis-text, #8c1d1d);
	}
	.kind-cluster {
		border-left-color: var(--accent-info-text, #1e3a5f);
	}
	.kind-continuation {
		border-left-color: var(--alt-primary, #2f4f4f);
	}
	.kind-inquiry {
		border-left-color: var(--accent-muted-text, #4b5563);
	}
	.branch-kind {
		display: inline-flex;
		font-family: var(--font-mono);
		font-size: 0.64rem;
		font-weight: 600;
		letter-spacing: 0.12em;
		text-transform: uppercase;
		padding: 0.14rem 0.55rem;
		border: 1px solid var(--surface-border, #c8c8c8);
		color: var(--alt-slate, #666);
	}
	.kind-contradiction .branch-kind {
		color: var(--accent-emphasis-text, #8c1d1d);
	}
	.kind-cluster .branch-kind {
		color: var(--accent-info-text, #1e3a5f);
	}
	.kind-continuation .branch-kind {
		color: var(--alt-primary, #2f4f4f);
	}
	.kind-inquiry .branch-kind {
		color: var(--accent-muted-text, #4b5563);
	}
	.branch-title {
		font-family: var(--font-display);
		font-size: 1.02rem;
		font-weight: 600;
		line-height: 1.3;
		margin-top: 0.45rem;
		color: var(--alt-charcoal, #1a1a1a);
	}
	.branch-why {
		font-size: 0.83rem;
		color: var(--text-secondary, #333);
		line-height: 1.5;
		margin: 0.3rem 0 0;
	}
	.branch-evidence {
		font-family: var(--font-mono);
		font-size: 0.67rem;
		color: var(--alt-ash, #999);
		margin-top: 0.45rem;
		letter-spacing: 0.02em;
	}
</style>
