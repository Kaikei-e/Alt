<script lang="ts">
import type {
	BranchData,
	BranchResolution,
} from "$lib/connect/knowledge_trail";

interface Props {
	branches: BranchData[];
	onResolve: (branchKey: string, resolution: BranchResolution) => void;
}

const { branches, onResolve }: Props = $props();

// Branches are proposals layered on the spine, not the main column. Cap them so
// they never bury the trail; the rest are one click away.
const VISIBLE = 3;
let showAll = $state(false);
const visible = $derived(showAll ? branches : branches.slice(0, VISIBLE));
const hiddenCount = $derived(Math.max(0, branches.length - VISIBLE));

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
		<h2 class="branches-heading">Suggested branches <span class="branches-count">({branches.length})</span></h2>
		{#each visible as branch (branch.branchKey)}
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
				<div class="branch-actions">
					<button
						class="take-path"
						data-testid="branch-take"
						onclick={() => onResolve(branch.branchKey, "taken")}
					>
						Take this path
					</button>
					<button
						class="dismiss-path"
						data-testid="branch-dismiss"
						onclick={() => onResolve(branch.branchKey, "dismissed")}
					>
						Dismiss
					</button>
				</div>
			</article>
		{/each}
		{#if hiddenCount > 0 && !showAll}
			<button
				class="branches-more"
				data-testid="branches-show-more"
				onclick={() => (showAll = true)}
			>
				Show {hiddenCount} more
			</button>
		{/if}
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
	.branches-count {
		font-family: var(--font-mono);
		font-size: 0.75rem;
		font-weight: 500;
		color: var(--alt-ash, #999);
	}
	.branches-more {
		border: 1px solid var(--chip-border, #d0c8bb);
		background: var(--action-surface, #ebe8e1);
		color: var(--interactive-text, #2f4f4f);
		font-family: var(--font-body);
		font-size: 0.82rem;
		padding: 0.45rem 0.85rem;
		cursor: pointer;
		margin-top: 0.2rem;
	}
	.branches-more:hover {
		background: var(--surface-hover, #f3f1ed);
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
	.branch-actions {
		display: flex;
		gap: 0.5rem;
		margin-top: 0.65rem;
	}
	.take-path {
		border: 1px solid var(--alt-primary, #2f4f4f);
		background: var(--alt-primary, #2f4f4f);
		color: var(--surface-bg, #faf9f7);
		padding: 0.34rem 0.8rem;
		font-family: var(--font-body);
		font-size: 0.8rem;
		font-weight: 600;
		cursor: pointer;
	}
	.take-path:hover {
		background: var(--interactive-text-hover, #223b3b);
	}
	.dismiss-path {
		border: 1px solid var(--chip-border, #d0c8bb);
		background: transparent;
		color: var(--alt-slate, #666);
		padding: 0.34rem 0.8rem;
		font-family: var(--font-body);
		font-size: 0.8rem;
		cursor: pointer;
	}
	.dismiss-path:hover {
		background: var(--surface-hover, #f3f1ed);
		color: var(--alt-charcoal, #1a1a1a);
	}
</style>
