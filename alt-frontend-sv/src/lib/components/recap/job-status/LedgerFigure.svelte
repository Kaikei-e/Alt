<script lang="ts">
interface Props {
	label: string;
	value: string | number;
	subtitle?: string;
	delta?: number | null;
	deltaUnit?: string;
}

let {
	label,
	value,
	subtitle,
	delta = null,
	deltaUnit = "%",
}: Props = $props();

const deltaGlyph = $derived(() => {
	if (delta === null || delta === 0) return "●";
	return delta > 0 ? "▲" : "▼";
});

const deltaInk = $derived(() => {
	if (delta === null || delta === 0) return "muted";
	return delta > 0 ? "success" : "error";
});

const deltaText = $derived(() => {
	if (delta === null) return "";
	return `${Math.abs(delta).toFixed(1)}${deltaUnit}`;
});
</script>

<dl class="ledger-figure" data-role="ledger-figure">
	<dt class="label">{label}</dt>
	<dd class="value tabular-nums">{value}</dd>
	{#if subtitle}
		<dd class="subtitle">{subtitle}</dd>
	{/if}
	{#if delta !== null}
		<dd class="delta" data-ink={deltaInk()}>
			<span class="glyph" aria-hidden="true">{deltaGlyph()}</span>
			<span class="text">{deltaText()}</span>
		</dd>
	{/if}
</dl>

<style>
	.ledger-figure {
		display: flex;
		flex-direction: column;
		gap: 0.35rem;
		margin: 0;
		padding: 0;
	}

	.label {
		font-family: var(--font-body);
		font-size: 0.6rem;
		font-weight: 600;
		letter-spacing: 0.1em;
		text-transform: uppercase;
		color: var(--alt-ash);
		margin: 0;
	}

	.value {
		font-family: var(--font-display);
		font-size: clamp(1.4rem, 3vw, 1.75rem);
		font-weight: 700;
		line-height: 1;
		color: var(--alt-charcoal);
		margin: 0;
	}

	.subtitle {
		font-family: var(--font-body);
		font-size: 0.75rem;
		color: var(--alt-slate);
		margin: 0;
	}

	.delta {
		display: inline-flex;
		align-items: baseline;
		gap: 0.3rem;
		font-family: var(--font-mono);
		font-size: 0.7rem;
		margin: 0;
	}

	.delta[data-ink="success"] {
		color: var(--alt-success);
	}

	.delta[data-ink="error"] {
		color: var(--alt-error);
	}

	.delta[data-ink="muted"] {
		color: var(--alt-ash);
	}

	.glyph {
		font-size: 0.75rem;
	}
</style>
