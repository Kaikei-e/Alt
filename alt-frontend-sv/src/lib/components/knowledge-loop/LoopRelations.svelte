<script lang="ts">
import type {
	RelationData,
	RelationKindName,
} from "$lib/connect/knowledge_loop";

/**
 * ADR-000937 relation-set — the always-present Orient surface. Rendered on both
 * the active-entry workspace and the secondary LoopEntryTile so the relation
 * set reaches the user regardless of which entry is in focus. Alt-Paper idiom:
 * a thin left rule whose colour encodes the relation's State, so the
 * open → advancing → advanced transition reads as the visible "loop closed".
 */

let { relations = [] }: { relations?: RelationData[] } = $props();

function relationKindLabel(kind: RelationKindName): string {
	switch (kind) {
		case "continuation":
			return "Continues";
		case "contradiction":
			return "Challenges";
		case "cluster":
			return "Extends";
		case "inquiry":
			return "Answers";
		default:
			return "Relates";
	}
}
</script>

{#if relations.length > 0}
	<ul class="relations" aria-label="How this connects to what you know">
		{#each relations as relation (relation.kind + relation.targetRef)}
			<li
				class="relation relation--{relation.state}"
				data-testid="loop-relation"
				data-relation-kind={relation.kind}
				data-relation-state={relation.state}
			>
				<span class="relation-kind">{relationKindLabel(relation.kind)}</span>
				<span class="relation-why">{relation.whyText}</span>
			</li>
		{/each}
	</ul>
{/if}

<style>
	.relations {
		list-style: none;
		margin: 0.7rem 0 0;
		padding: 0.55rem 0 0;
		border-top: 1px solid var(--surface-border, #c8c8c8);
		display: grid;
		gap: 0.4rem;
	}
	.relation {
		display: grid;
		grid-template-columns: auto 1fr;
		gap: 0.5rem;
		align-items: baseline;
		padding-left: 0.6rem;
		border-left: 2px solid var(--alt-ash, #999);
	}
	.relation-kind {
		font-family: var(--font-body, "Source Sans 3", system-ui, sans-serif);
		font-size: 0.6rem;
		font-weight: 700;
		letter-spacing: 0.12em;
		text-transform: uppercase;
		color: var(--alt-slate, #666);
		white-space: nowrap;
	}
	.relation-why {
		font-family: var(--font-body, "Source Sans 3", system-ui, sans-serif);
		font-size: 0.82rem;
		line-height: 1.5;
		color: var(--alt-charcoal, #1a1a1a);
	}
	/* State ladder — the visible "loop closed" cue. */
	.relation--open {
		border-left-color: var(--alt-ash, #999);
	}
	.relation--advancing {
		border-left-color: var(--alt-sand, #d4a574);
	}
	.relation--advanced {
		border-left-color: var(--alt-primary, #2f4f4f);
	}
	.relation--resolved {
		border-left-color: var(--alt-slate, #666);
		opacity: 0.7;
	}
</style>
