<script lang="ts">
import type { TagTrailTag } from "$lib/schema/tagTrail";

interface Props {
	tag: TagTrailTag;
	isSelected?: boolean;
	onclick: (tag: TagTrailTag) => void;
}

const { tag, isSelected = false, onclick }: Props = $props();

const handleClick = () => {
	onclick(tag);
};
</script>

<button
	type="button"
	class="index-entry"
	class:index-entry--active={isSelected}
	onclick={handleClick}
	aria-pressed={isSelected}
	data-testid="tag-chip-{tag.id}"
	data-role="index-entry"
>
	{tag.name}
</button>

<style>
	.index-entry {
		display: inline-flex;
		align-items: center;
		padding: 0.4rem 0.65rem;
		min-height: 44px;
		flex-shrink: 0;

		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.7rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.06em;

		color: var(--alt-slate, #666);
		background: transparent;
		border: 1px solid var(--surface-border, #c8c8c8);

		transition: background 0.15s, color 0.15s, border-color 0.15s;
		cursor: pointer;
	}

	.index-entry:hover {
		background: var(--alt-charcoal, #1a1a1a);
		color: var(--surface-bg, #faf9f7);
		border-color: var(--alt-charcoal, #1a1a1a);
	}

	.index-entry--active {
		background: var(--alt-charcoal, #1a1a1a);
		color: var(--surface-bg, #faf9f7);
		border-color: var(--alt-charcoal, #1a1a1a);
	}
</style>
