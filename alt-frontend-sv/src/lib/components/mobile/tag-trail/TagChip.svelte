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
	class="px-2.5 py-1 rounded-full text-xs font-medium transition-all min-h-[36px] active:scale-95 flex-shrink-0"
	class:bg-primary={isSelected}
	class:text-primary-foreground={isSelected}
	style={isSelected
		? ""
		: "background: var(--muted); color: var(--text-secondary);"}
	onclick={handleClick}
	aria-pressed={isSelected}
	data-testid="tag-chip-{tag.id}"
>
	{tag.name}
</button>
