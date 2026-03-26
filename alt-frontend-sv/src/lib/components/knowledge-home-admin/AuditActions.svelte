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

<div
	class="rounded-lg border-2 p-4"
	style="background: var(--surface-bg); border-color: var(--border-primary);"
	data-testid="audit-actions-panel"
>
	<h3 class="mb-3 text-sm font-semibold" style="color: var(--text-primary);">
		Run Projection Audit
	</h3>
	<p class="mb-4 text-xs" style="color: var(--text-secondary);">
		Sample items from the projection and verify correctness against the event log.
		Detects drift in item count, scores, and empty summaries.
	</p>
	<div class="flex flex-wrap items-end gap-3">
		<div class="flex flex-col gap-1">
			<label
				for="audit-projection-name"
				class="text-xs font-medium"
				style="color: var(--text-secondary);"
			>
				Projection Name
			</label>
			<input
				id="audit-projection-name"
				type="text"
				bind:value={projectionName}
				class="rounded border px-2 py-1.5 text-xs"
				style="background: var(--surface-bg); border-color: var(--border-primary); color: var(--text-primary);"
			/>
		</div>
		<div class="flex flex-col gap-1">
			<label
				for="audit-projection-version"
				class="text-xs font-medium"
				style="color: var(--text-secondary);"
			>
				Projection Version
			</label>
			<input
				id="audit-projection-version"
				type="text"
				bind:value={projectionVersion}
				class="rounded border px-2 py-1.5 text-xs"
				style="background: var(--surface-bg); border-color: var(--border-primary); color: var(--text-primary);"
			/>
		</div>
		<div class="flex flex-col gap-1">
			<label
				for="audit-sample-size"
				class="text-xs font-medium"
				style="color: var(--text-secondary);"
			>
				Sample Size
			</label>
			<input
				id="audit-sample-size"
				type="number"
				min="1"
				max="1000"
				bind:value={sampleSize}
				class="w-24 rounded border px-2 py-1.5 text-xs"
				style="background: var(--surface-bg); border-color: var(--border-primary); color: var(--text-primary);"
			/>
		</div>
		<button
			type="button"
			class="rounded px-4 py-1.5 text-xs font-medium text-white transition-opacity"
			style="background: var(--accent-blue, #3b82f6);"
			disabled={inFlight}
			onclick={handleSubmit}
		>
			{inFlight ? "Running..." : "Run Audit"}
		</button>
	</div>
</div>
