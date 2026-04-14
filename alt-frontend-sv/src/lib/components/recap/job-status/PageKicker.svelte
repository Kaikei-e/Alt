<script lang="ts">
interface Props {
	kicker: string;
	title: string;
	lede?: string;
	testId?: string;
}

let { kicker, title, lede, testId }: Props = $props();
</script>

<header class="page-kicker" data-role="page-kicker" data-testid={testId}>
	<p class="kicker">{kicker}</p>
	<h1 class="title">{title}</h1>
	{#if lede}
		<p class="lede">{lede}</p>
	{/if}
	<div class="rule" aria-hidden="true"></div>
</header>

<style>
	.page-kicker {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		padding-bottom: 1rem;
	}

	.kicker {
		font-family: var(--font-body);
		font-size: 0.65rem;
		font-weight: 600;
		letter-spacing: 0.1em;
		text-transform: uppercase;
		color: var(--alt-ash);
		margin: 0;
	}

	.title {
		font-family: var(--font-display);
		font-size: clamp(1.6rem, 4vw, 2.2rem);
		font-weight: 700;
		line-height: 1.1;
		letter-spacing: -0.01em;
		color: var(--alt-charcoal);
		margin: 0;
	}

	.lede {
		font-family: var(--font-body);
		font-size: 0.95rem;
		line-height: 1.6;
		color: var(--alt-slate);
		max-width: 65ch;
		margin: 0;
	}

	.rule {
		height: 1px;
		background: var(--surface-border);
		margin-top: 0.5rem;
	}
</style>
