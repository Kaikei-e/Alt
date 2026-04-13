<script lang="ts">
type Props = {
	index: number; // 0-based citation index
	onSelect?: (index: number) => void;
	active?: boolean;
};

let { index, onSelect, active = false }: Props = $props();

function pad2(n: number): string {
	return String(n).padStart(2, "0");
}

function handle(event: MouseEvent | KeyboardEvent) {
	if (event instanceof KeyboardEvent) {
		if (event.key !== "Enter" && event.key !== " ") return;
		event.preventDefault();
	}
	onSelect?.(index);
}
</script>

<button
	type="button"
	class="citation-chip"
	class:is-active={active}
	aria-label="Citation {index + 1}"
	onclick={handle}
	onkeydown={handle}
>
	[{pad2(index + 1)}]
</button>

<style>
	.citation-chip {
		display: inline-flex;
		align-items: center;
		font-family: var(--font-mono, "IBM Plex Mono", monospace);
		font-size: 0.7rem;
		font-weight: 600;
		color: var(--text-muted, #999);
		background: transparent;
		border: 1px solid transparent;
		padding: 0.05rem 0.3rem;
		margin: 0 0.15rem;
		cursor: pointer;
		vertical-align: super;
		line-height: 1;
		transition: color 120ms ease, border-color 120ms ease,
			background 120ms ease;
	}

	.citation-chip:hover,
	.citation-chip:focus-visible,
	.citation-chip.is-active {
		color: var(--accent-primary, #2f4f4f);
		border-color: var(--surface-border, #c8c8c8);
		background: var(--surface-hover, rgba(0, 0, 0, 0.04));
		outline: none;
	}
</style>
