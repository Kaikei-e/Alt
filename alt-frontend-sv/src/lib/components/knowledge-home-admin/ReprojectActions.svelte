<script lang="ts">
let {
	onStart,
	inFlight = false,
}: {
	onStart: (
		mode: string,
		fromVersion: string,
		toVersion: string,
		rangeStart?: string,
		rangeEnd?: string,
	) => void;
	inFlight?: boolean;
} = $props();

let mode = $state("dry_run");
let fromVersion = $state("");
let toVersion = $state("");
let rangeStart = $state("");
let rangeEnd = $state("");

const modes = [
	{ value: "dry_run", label: "Dry Run" },
	{ value: "user_subset", label: "User Subset" },
	{ value: "time_range", label: "Time Range" },
	{ value: "full", label: "Full" },
];

const showRange = $derived(mode === "time_range");
const canSubmit = $derived(
	!inFlight &&
		fromVersion.trim() !== "" &&
		toVersion.trim() !== "" &&
		(!showRange || (rangeStart.trim() !== "" && rangeEnd.trim() !== "")),
);

const handleSubmit = () => {
	if (!canSubmit) return;
	onStart(
		mode,
		fromVersion.trim(),
		toVersion.trim(),
		showRange ? rangeStart.trim() : undefined,
		showRange ? rangeEnd.trim() : undefined,
	);
};
</script>

<div class="panel" data-role="reproject-actions">
	<h3 class="section-heading">Start New Reproject</h3>
	<div class="heading-rule"></div>

	<div class="form-grid">
		<div class="form-field">
			<label for="reproject-mode" class="field-label">Mode</label>
			<select
				id="reproject-mode"
				class="field-input"
				bind:value={mode}
				disabled={inFlight}
			>
				{#each modes as m (m.value)}
					<option value={m.value}>{m.label}</option>
				{/each}
			</select>
		</div>

		<div class="form-field">
			<label for="reproject-from-version" class="field-label">From Version</label>
			<input
				id="reproject-from-version"
				type="text"
				class="field-input field-mono"
				placeholder="e.g. v1"
				bind:value={fromVersion}
				disabled={inFlight}
			/>
		</div>

		<div class="form-field">
			<label for="reproject-to-version" class="field-label">To Version</label>
			<input
				id="reproject-to-version"
				type="text"
				class="field-input field-mono"
				placeholder="e.g. v2"
				bind:value={toVersion}
				disabled={inFlight}
			/>
		</div>

		<div class="form-field form-field-action">
			<button
				class="submit-btn"
				disabled={!canSubmit}
				onclick={handleSubmit}
			>
				{inFlight ? "Starting..." : "Start Reproject"}
			</button>
		</div>
	</div>

	{#if showRange}
		<div class="range-grid">
			<div class="form-field">
				<label for="reproject-range-start" class="field-label">Range Start (RFC3339)</label>
				<input
					id="reproject-range-start"
					type="text"
					class="field-input field-mono"
					placeholder="2026-03-01T00:00:00Z"
					bind:value={rangeStart}
					disabled={inFlight}
				/>
			</div>
			<div class="form-field">
				<label for="reproject-range-end" class="field-label">Range End (RFC3339)</label>
				<input
					id="reproject-range-end"
					type="text"
					class="field-input field-mono"
					placeholder="2026-03-18T00:00:00Z"
					bind:value={rangeEnd}
					disabled={inFlight}
				/>
			</div>
		</div>
	{/if}
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

	.form-grid {
		display: grid;
		grid-template-columns: repeat(4, 1fr);
		gap: 0.75rem;
	}

	.range-grid {
		display: grid;
		grid-template-columns: 1fr 1fr;
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

	.field-input:disabled {
		opacity: 0.5;
	}

	.field-mono {
		font-family: var(--font-mono);
		font-size: 0.8rem;
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

	@media (max-width: 768px) {
		.form-grid {
			grid-template-columns: 1fr 1fr;
		}

		.range-grid {
			grid-template-columns: 1fr;
		}
	}
</style>
