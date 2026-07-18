<script lang="ts">
import type {
	BranchData,
	ResolveBranchHandler,
} from "$lib/connect/knowledge_trail";

interface Props {
	branch: BranchData;
	/** testid applied to the card root; callers pick the surface-specific value. */
	testId: string;
	onResolve: ResolveBranchHandler;
}

const { branch, testId, onResolve }: Props = $props();

// Dismiss is a one-tap reason, not a modal: the actions row swaps in place
// for the reason row (D28) rather than navigating away from the card.
let dismissing = $state(false);

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

// Canonical one-tap dismiss reasons (D28) — recorded on the resolution event
// for planner learning, never required.
const DISMISS_REASONS: { id: string; label: string }[] = [
	{ id: "not_following_topic", label: "Not following this topic" },
	{ id: "already_known", label: "Already knew this" },
	{ id: "wrong_relation", label: "Wrong connection" },
];

function take() {
	onResolve(branch.branchKey, "taken", branch.targetItemKey);
}

function dismissWithReason(reason?: string) {
	onResolve(branch.branchKey, "dismissed", undefined, reason);
	dismissing = false;
}
</script>

<article
	class="branch-card kind-{branch.relationKind}"
	data-testid={testId}
	data-relation-kind={branch.relationKind}
>
	<span class="branch-kind">{kindLabel(branch.relationKind)}</span>
	<div class="branch-title">{branch.targetTitle || branch.targetItemKey}</div>
	<p class="branch-why">{branch.why}</p>
	<div class="branch-evidence">
		evidence: {evidenceSummary(branch)} &nbsp;·&nbsp; confidence: {branch.confidence}
	</div>
	{#if dismissing}
		<div class="branch-dismiss-reasons" data-testid="branch-dismiss-reasons">
			{#each DISMISS_REASONS as reason (reason.id)}
				<button
					type="button"
					class="dismiss-reason"
					data-testid="branch-dismiss-reason-{reason.id}"
					onclick={() => dismissWithReason(reason.id)}
				>
					{reason.label}
				</button>
			{/each}
			<button
				type="button"
				class="dismiss-reason dismiss-reason--plain"
				data-testid="branch-dismiss-plain"
				onclick={() => dismissWithReason(undefined)}
			>
				Just dismiss
			</button>
		</div>
	{:else}
		<div class="branch-actions">
			<button
				type="button"
				class="take-path"
				data-testid="branch-take"
				onclick={take}
			>
				Take this path
			</button>
			<button
				type="button"
				class="dismiss-path"
				data-testid="branch-dismiss"
				onclick={() => (dismissing = true)}
			>
				Dismiss
			</button>
		</div>
	{/if}
</article>

<style>
	.branch-card {
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
	.branch-actions,
	.branch-dismiss-reasons {
		display: flex;
		gap: 0.5rem;
		margin-top: 0.65rem;
		flex-wrap: wrap;
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
	.dismiss-path,
	.dismiss-reason {
		border: 1px solid var(--chip-border, #d0c8bb);
		background: transparent;
		color: var(--alt-slate, #666);
		padding: 0.34rem 0.8rem;
		font-family: var(--font-body);
		font-size: 0.8rem;
		cursor: pointer;
	}
	.dismiss-path:hover,
	.dismiss-reason:hover {
		background: var(--surface-hover, #f3f1ed);
		color: var(--alt-charcoal, #1a1a1a);
	}
	.dismiss-reason--plain {
		font-style: italic;
	}
</style>
