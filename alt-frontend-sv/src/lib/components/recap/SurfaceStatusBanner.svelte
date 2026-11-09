<script lang="ts">
import type { SurfaceStatus } from "#lib/domain/recap/types.js";
import { evaluateSurfaceStatus } from "#lib/utils/surface-status.js";

interface Props {
	surfaceStatus?: SurfaceStatus | null;
	degradedTestId?: string;
}

const { surfaceStatus = null, degradedTestId }: Props = $props();

const evaluation = $derived(evaluateSurfaceStatus(surfaceStatus));
const statusNotice = $derived(evaluation.statusNotice);
const degradationNotices = $derived(evaluation.degradationNotices);
const infoNotice = $derived(
	statusNotice?.tone === "info" ? statusNotice : null,
);
const alertStatusNotice = $derived(
	statusNotice && statusNotice.tone !== "info" ? statusNotice : null,
);
const hasAlert = $derived(
	Boolean(alertStatusNotice) || degradationNotices.length > 0,
);
const alertTone = $derived(
	alertStatusNotice?.tone === "error" ? "error" : "warning",
);
</script>

{#if infoNotice || hasAlert}
	<div class="surface-status-stack">
		{#if infoNotice}
			<div class="surface-status-banner" data-tone="info" role="status">
				<p class="surface-status-text">{infoNotice.text}</p>
			</div>
		{/if}

		{#if hasAlert}
			<div
				class="surface-status-banner"
				data-tone={alertTone}
				role="alert"
				data-testid={degradationNotices.length > 0 ? degradedTestId : undefined}
			>
				{#if alertStatusNotice}
					<p class="surface-status-text">{alertStatusNotice.text}</p>
				{/if}
				{#each degradationNotices as notice}
					<p class="surface-status-text">{notice}</p>
				{/each}
			</div>
		{/if}
	</div>
{/if}

<style>
	.surface-status-stack {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		margin-bottom: 1rem;
	}

	.surface-status-banner {
		padding: 0.75rem 1rem;
		background: var(--surface-bg);
		border: 1px solid var(--surface-border);
		border-left: 3px solid var(--surface-border);
	}

	.surface-status-banner[data-tone="info"] {
		border-left-color: var(--accent-info-text, var(--alt-charcoal));
	}

	.surface-status-banner[data-tone="warning"] {
		border-left-color: var(--alt-warning);
	}

	.surface-status-banner[data-tone="error"] {
		border-left-color: var(--alt-error);
	}

	.surface-status-text {
		font-family: var(--font-body);
		font-size: 0.875rem;
		line-height: 1.5;
		color: var(--text-primary);
		margin: 0;
	}

	.surface-status-text + .surface-status-text {
		margin-top: 0.25rem;
	}
</style>
