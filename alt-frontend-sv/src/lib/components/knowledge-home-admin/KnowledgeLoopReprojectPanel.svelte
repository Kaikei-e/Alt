<script lang="ts">
import { onMount } from "svelte";
import type {
	KnowledgeLoopReprojectResult,
	KnowledgeLoopReprojectStatus,
} from "$lib/server/sovereign-admin";

/**
 * Operator panel for the Knowledge Loop full-reproject procedure
 * (docs/runbooks/knowledge-loop-reproject.md). Distinct from the
 * `ReprojectActions` panel above on this page, which targets the Knowledge
 * Home shadow/swap reproject — Knowledge Loop is a disposable projection
 * with TRUNCATE-and-rerun semantics, so the UX is a single destructive
 * "Reproject" action gated by an inline confirmation.
 *
 * The header surfaces the current WhyMappingVersion (the operator-facing
 * signal that placement / narrative rules changed in code) and the
 * projector's last_event_seq checkpoint. This is the "v7" the operator
 * cares about — it is NOT the same as the Knowledge Home projection_version
 * shown elsewhere on this page; the two systems are independent.
 *
 * Confirmation gate: the user must type "REPROJECT" exactly. Cheap human
 * speed bump that matches the runbook's intent and stops a stray click /
 * fat finger from wiping projections.
 */

let {
	onTrigger,
	onFetchStatus,
	disabled = false,
} = $props<{
	onTrigger: () => Promise<KnowledgeLoopReprojectResult>;
	onFetchStatus?: () => Promise<KnowledgeLoopReprojectStatus>;
	disabled?: boolean;
}>();

let confirmation = $state("");
let inFlight = $state(false);
let result = $state<KnowledgeLoopReprojectResult | null>(null);
let errorMessage = $state<string | null>(null);
let status = $state<KnowledgeLoopReprojectStatus | null>(null);
let statusError = $state<string | null>(null);

const armed = $derived(confirmation.trim() === "REPROJECT");

async function refreshStatus() {
	if (!onFetchStatus) return;
	try {
		status = await onFetchStatus();
		statusError = null;
	} catch (e) {
		statusError = e instanceof Error ? e.message : "unknown_error";
	}
}

onMount(() => {
	void refreshStatus();
});

async function run() {
	if (!armed || inFlight) return;
	inFlight = true;
	errorMessage = null;
	result = null;
	try {
		result = await onTrigger();
		confirmation = "";
		// Refresh the version + checkpoint readout so the operator can verify
		// the projector is advancing past zero again.
		void refreshStatus();
	} catch (e) {
		errorMessage = e instanceof Error ? e.message : "unknown_error";
	} finally {
		inFlight = false;
	}
}
</script>

<section class="panel" data-testid="knowledge-loop-reproject-panel">
	<header class="panel-head">
		<h3 class="panel-title">Knowledge Loop reproject</h3>
		<p class="panel-blurb">
			TRUNCATEs the three Knowledge Loop projection tables and resets the
			projector checkpoint. Dedupe table is preserved. The projector picks
			up from event_seq=0 on its next 5-second tick.
		</p>
		{#if status}
			<dl class="status-readout" data-testid="knowledge-loop-reproject-status">
				<dt>WhyMappingVersion</dt>
				<dd data-testid="knowledge-loop-reproject-status-version">
					v{status.why_mapping_version}
				</dd>
				<dt>Projector checkpoint</dt>
				<dd>event_seq {status.last_event_seq}</dd>
				<dt>Projector</dt>
				<dd>{status.projector_name}</dd>
			</dl>
			<p class="status-note">
				This is the projector's <em>code-side</em> version. It is not the
				same as the Knowledge Home <code>projection_version</code> shown
				elsewhere on this page.
			</p>
		{:else if statusError}
			<p class="status status--error">Status unavailable: {statusError}</p>
		{/if}
	</header>

	<div class="panel-body">
		<label for="kl-reproject-confirm" class="confirm-label">
			Type <code>REPROJECT</code> to enable
		</label>
		<input
			id="kl-reproject-confirm"
			class="confirm-input"
			type="text"
			autocomplete="off"
			spellcheck="false"
			bind:value={confirmation}
			disabled={inFlight || disabled}
			placeholder="REPROJECT"
			data-testid="knowledge-loop-reproject-confirm"
		/>
		<button
			type="button"
			class="run"
			class:armed
			disabled={!armed || inFlight || disabled}
			onclick={() => void run()}
			data-testid="knowledge-loop-reproject-run"
		>
			{inFlight ? "Reprojecting…" : "Reproject Knowledge Loop"}
		</button>
	</div>

	{#if errorMessage}
		<p class="status status--error" role="alert" data-testid="knowledge-loop-reproject-error">
			Reproject failed: {errorMessage}
		</p>
	{/if}

	{#if result?.ok}
		<dl class="result" data-testid="knowledge-loop-reproject-result">
			<dt>Entries truncated</dt>
			<dd>{result.entries_truncated}</dd>
			<dt>Session-state truncated</dt>
			<dd>{result.session_state_truncated}</dd>
			<dt>Surfaces truncated</dt>
			<dd>{result.surfaces_truncated}</dd>
			<dt>Checkpoint reset</dt>
			<dd>{result.checkpoint_reset ? "yes" : "no"}</dd>
		</dl>
		<p class="status status--ok">{result.projector_will_run_on_tick}</p>
	{/if}
</section>

<style>
	.panel {
		border: 1px solid var(--surface-border, #c8c8c8);
		background: var(--surface-bg, #faf9f7);
		padding: 1rem 1.1rem;
		display: grid;
		gap: 0.75rem;
	}
	.panel-head {
		border-bottom: 1px solid var(--surface-border, #c8c8c8);
		padding-bottom: 0.5rem;
	}
	.panel-title {
		font-family: var(--font-display, "Playfair Display", Georgia, serif);
		font-size: 1.05rem;
		font-weight: 700;
		margin: 0 0 0.25rem;
		color: var(--alt-charcoal, #1a1a1a);
	}
	.panel-blurb {
		margin: 0;
		font-family: var(--font-body, "Source Sans 3", system-ui, sans-serif);
		font-size: 0.78rem;
		line-height: 1.5;
		color: var(--alt-slate, #666);
	}
	.status-readout {
		display: grid;
		grid-template-columns: max-content 1fr;
		row-gap: 0.18rem;
		column-gap: 1rem;
		margin: 0.5rem 0 0.35rem;
		font-family: var(--font-mono, "IBM Plex Mono", ui-monospace, monospace);
		font-size: 0.74rem;
	}
	.status-readout dt {
		color: var(--alt-ash, #999);
		letter-spacing: 0.05em;
	}
	.status-readout dd {
		margin: 0;
		color: var(--alt-charcoal, #1a1a1a);
	}
	.status-note {
		margin: 0;
		font-family: var(--font-body, "Source Sans 3", system-ui, sans-serif);
		font-size: 0.7rem;
		color: var(--alt-ash, #999);
		line-height: 1.5;
	}
	.status-note code {
		font-family: var(--font-mono, "IBM Plex Mono", ui-monospace, monospace);
		font-size: 0.66rem;
		color: var(--alt-slate, #666);
	}
	.panel-body {
		display: grid;
		gap: 0.4rem;
	}
	.confirm-label {
		font-family: var(--font-mono, "IBM Plex Mono", ui-monospace, monospace);
		font-size: 0.66rem;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--alt-ash, #999);
	}
	.confirm-label code {
		font-family: inherit;
		color: var(--alt-charcoal, #1a1a1a);
	}
	.confirm-input {
		font-family: var(--font-mono, "IBM Plex Mono", ui-monospace, monospace);
		font-size: 0.85rem;
		padding: 0.35rem 0.55rem;
		border: 1px solid var(--surface-border, #c8c8c8);
		background: var(--surface-2, #f5f4f1);
		color: var(--alt-charcoal, #1a1a1a);
		max-width: 28ch;
	}
	.confirm-input:focus-visible {
		outline: 2px solid var(--alt-charcoal, #1a1a1a);
		outline-offset: 1px;
	}
	.run {
		appearance: none;
		font-family: var(--font-mono, "IBM Plex Mono", ui-monospace, monospace);
		font-size: 0.78rem;
		letter-spacing: 0.05em;
		padding: 0.4rem 0.9rem;
		border: 1px solid var(--surface-border, #c8c8c8);
		background: transparent;
		color: var(--alt-ash, #999);
		cursor: not-allowed;
		max-width: max-content;
	}
	.run.armed:not(:disabled) {
		border-color: var(--alt-terracotta, #b85450);
		color: var(--alt-terracotta, #b85450);
		cursor: pointer;
	}
	.run.armed:not(:disabled):hover {
		background: var(--surface-2, #f5f4f1);
	}
	.run:focus-visible {
		outline: 2px solid var(--alt-charcoal, #1a1a1a);
		outline-offset: 2px;
	}
	.status {
		font-family: var(--font-mono, "IBM Plex Mono", ui-monospace, monospace);
		font-size: 0.72rem;
		margin: 0;
		padding-top: 0.35rem;
		border-top: 1px dotted var(--surface-border, #c8c8c8);
	}
	.status--ok {
		color: var(--alt-slate, #666);
	}
	.status--error {
		color: var(--alt-terracotta, #b85450);
	}
	.result {
		display: grid;
		grid-template-columns: max-content 1fr;
		row-gap: 0.2rem;
		column-gap: 1rem;
		font-family: var(--font-mono, "IBM Plex Mono", ui-monospace, monospace);
		font-size: 0.74rem;
		margin: 0;
		padding-top: 0.35rem;
		border-top: 1px dotted var(--surface-border, #c8c8c8);
	}
	.result dt {
		color: var(--alt-ash, #999);
		letter-spacing: 0.05em;
	}
	.result dd {
		margin: 0;
		color: var(--alt-charcoal, #1a1a1a);
	}
</style>
