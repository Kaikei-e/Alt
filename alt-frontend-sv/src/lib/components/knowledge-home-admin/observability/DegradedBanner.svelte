<script lang="ts">
let {
	message = "Observability source unreachable.",
	hint = "Showing the last snapshot we received. The hook is reconnecting in the background.",
}: {
	message?: string;
	hint?: string;
} = $props();
</script>

<div class="banner" role="status" aria-live="polite">
	<span class="rule" aria-hidden="true"></span>
	<span class="msg">{message}</span>
	<span class="hint">{hint}</span>
</div>

<style>
	.banner {
		display: grid;
		grid-template-columns: 2px 1fr;
		grid-template-areas: "rule msg" "rule hint";
		column-gap: 0.75rem;
		row-gap: 0.15rem;
		padding: 0.45rem 0 0.45rem 0.5rem;
		font-size: 0.78rem;
		border-top: 1px solid var(--obs-rule, var(--surface-border));
		border-bottom: 1px solid var(--obs-rule, var(--surface-border));
	}

	.rule {
		grid-area: rule;
		background: var(--obs-warn, var(--alt-warning));
		border-radius: 1px;
	}

	.msg {
		grid-area: msg;
		color: var(--alt-charcoal);
		font-weight: 500;
	}

	.hint {
		grid-area: hint;
		color: var(--obs-muted, var(--alt-ash));
	}
</style>
