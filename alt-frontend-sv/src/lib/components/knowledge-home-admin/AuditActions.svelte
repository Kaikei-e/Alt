<script lang="ts">
interface Props {
	onRunAudit: (
		projectionName: string,
		projectionVersion: string,
		sampleSize: number,
	) => void;
	inFlight?: boolean;
}

let { onRunAudit, inFlight = false }: Props = $props();

let projectionName = $state("knowledge_home");
let projectionVersion = $state("1");
let sampleSize = $state(100);

const handleSubmit = () => {
	if (!projectionName || !projectionVersion || sampleSize <= 0) return;
	onRunAudit(projectionName, projectionVersion, sampleSize);
};
</script>

<div class="panel" data-role="audit-actions">
	<h3 class="section-heading">Run Projection Audit</h3>
	<div class="heading-rule"></div>
	<p class="panel-desc">
		Sample items from the projection and verify correctness against the event log.
		Detects drift in item count, scores, and empty summaries.
	</p>
	<div class="form-row">
		<div class="form-field">
			<label for="audit-projection-name" class="field-label">Projection Name</label>
			<input
				id="audit-projection-name"
				type="text"
				class="field-input field-mono"
				bind:value={projectionName}
			/>
		</div>
		<div class="form-field">
			<label for="audit-projection-version" class="field-label">Projection Version</label>
			<input
				id="audit-projection-version"
				type="text"
				class="field-input field-mono"
				bind:value={projectionVersion}
			/>
		</div>
		<div class="form-field">
			<label for="audit-sample-size" class="field-label">Sample Size</label>
			<input
				id="audit-sample-size"
				type="number"
				min="1"
				max="1000"
				class="field-input field-mono field-narrow"
				bind:value={sampleSize}
			/>
		</div>
		<div class="form-field form-field-action">
			<button
				class="submit-btn"
				disabled={inFlight}
				onclick={handleSubmit}
			>
				{inFlight ? "Running..." : "Run Audit"}
			</button>
		</div>
	</div>
</div>

<style>
	.panel {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
		padding: 1rem;
		border: 1px solid var(--surface-border);
		background: var(--surface-bg);
	}

	.section-heading {
		font-family: var(--font-display);
		font-size: 1.05rem;
		font-weight: 700;
		line-height: 1.3;
		color: var(--alt-charcoal);
		margin: 0;
	}

	.heading-rule {
		height: 1px;
		background: var(--surface-border);
		margin-bottom: 0.25rem;
	}

	.panel-desc {
		font-family: var(--font-body);
		font-size: 0.75rem;
		color: var(--alt-slate);
		margin: 0;
		max-width: 60ch;
	}

	.form-row {
		display: flex;
		flex-wrap: wrap;
		align-items: flex-end;
		gap: 0.75rem;
	}

	.form-field {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}

	.form-field-action {
		display: flex;
		align-items: flex-end;
	}

	.field-label {
		font-size: 0.65rem;
		font-weight: 600;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--alt-ash);
	}

	.field-input {
		border: 1px solid var(--surface-border);
		background: transparent;
		color: var(--alt-charcoal);
		font-family: var(--font-body);
		font-size: 1rem;
		padding: 0.35rem 0.5rem;
		transition: border-color 0.15s;
	}

	.field-input:focus {
		border-color: var(--alt-charcoal);
		outline: none;
	}

	.field-mono {
		font-family: var(--font-mono);
		font-size: 0.8rem;
	}

	.field-narrow {
		width: 6rem;
	}

	.submit-btn {
		border: 1.5px solid var(--alt-charcoal);
		background: var(--alt-charcoal);
		color: var(--surface-bg);
		font-family: var(--font-body);
		font-size: 0.65rem;
		font-weight: 600;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		padding: 0.4rem 0.75rem;
		cursor: pointer;
		transition: background 0.15s, color 0.15s;
		white-space: nowrap;
	}

	.submit-btn:hover:not(:disabled) {
		background: transparent;
		color: var(--alt-charcoal);
	}

	.submit-btn:disabled {
		opacity: 0.4;
		cursor: not-allowed;
	}
</style>
