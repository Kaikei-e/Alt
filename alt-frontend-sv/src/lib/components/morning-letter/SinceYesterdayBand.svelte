<script lang="ts">
type Props = {
	targetDate: string;
	throughLine: string;
	onNavigate?: (targetDate: string) => void;
};

let { targetDate, throughLine, onNavigate }: Props = $props();

function handleClick() {
	onNavigate?.(targetDate);
}
</script>

<aside class="since-yesterday" data-role="since-yesterday">
	<div class="since-label">Since yesterday</div>
	<button class="since-link" type="button" onclick={handleClick}>
		<span class="since-date">{targetDate}</span>
		<span class="since-arrow" aria-hidden="true">→</span>
		<span class="since-line">{throughLine}</span>
	</button>
</aside>

<style>
	.since-yesterday {
		margin: 1rem 0 1.5rem;
		padding: 0.85rem 1rem;
		background: var(--alt-paper-dark, #eae5da);
		border-left: 4px solid var(--alt-ink, #1a1a1a);
	}

	.since-label {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.62rem;
		letter-spacing: 0.14em;
		text-transform: uppercase;
		color: var(--alt-ash, #666);
		margin-bottom: 0.3rem;
	}

	.since-link {
		display: flex;
		flex-wrap: wrap;
		gap: 0.4rem;
		align-items: baseline;
		background: transparent;
		border: none;
		padding: 0;
		text-align: left;
		cursor: pointer;
		color: inherit;
		width: 100%;
	}

	.since-date {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.78rem;
		font-weight: 600;
		color: var(--alt-ink, #1a1a1a);
	}

	.since-arrow {
		color: var(--alt-vermillion, #a83232);
		font-weight: 700;
	}

	.since-line {
		font-family: var(--font-display, "Playfair Display", serif);
		font-size: 0.95rem;
		line-height: 1.35;
		color: var(--alt-charcoal, #1a1a1a);
		flex: 1 1 60%;
		min-width: 16rem;
	}

	.since-link:hover .since-line {
		text-decoration: underline;
		text-underline-offset: 3px;
	}
</style>
