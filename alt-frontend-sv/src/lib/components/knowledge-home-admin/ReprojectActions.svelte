<script lang="ts">
import { Button } from "$lib/components/ui/button";

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

<div
	class="flex flex-col gap-4 rounded-lg border-2 p-4"
	style="background: var(--surface-bg); border-color: var(--border-primary);"
>
	<h3 class="text-sm font-semibold" style="color: var(--text-primary);">
		Start New Reproject
	</h3>

	<div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
		<div class="flex flex-col gap-1">
			<label class="text-xs font-medium" style="color: var(--text-secondary);">
				Mode
			</label>
			<select
				class="rounded-md border px-3 py-1.5 text-sm"
				style="background: var(--surface-bg); border-color: var(--surface-border, #d1d5db); color: var(--text-primary);"
				bind:value={mode}
				disabled={inFlight}
			>
				{#each modes as m (m.value)}
					<option value={m.value}>{m.label}</option>
				{/each}
			</select>
		</div>

		<div class="flex flex-col gap-1">
			<label class="text-xs font-medium" style="color: var(--text-secondary);">
				From Version
			</label>
			<input
				type="text"
				class="rounded-md border px-3 py-1.5 text-sm font-mono"
				style="background: var(--surface-bg); border-color: var(--surface-border, #d1d5db); color: var(--text-primary);"
				placeholder="e.g. v1"
				bind:value={fromVersion}
				disabled={inFlight}
			/>
		</div>

		<div class="flex flex-col gap-1">
			<label class="text-xs font-medium" style="color: var(--text-secondary);">
				To Version
			</label>
			<input
				type="text"
				class="rounded-md border px-3 py-1.5 text-sm font-mono"
				style="background: var(--surface-bg); border-color: var(--surface-border, #d1d5db); color: var(--text-primary);"
				placeholder="e.g. v2"
				bind:value={toVersion}
				disabled={inFlight}
			/>
		</div>

		<div class="flex items-end">
			<Button
				variant="default"
				size="sm"
				disabled={!canSubmit}
				onclick={handleSubmit}
			>
				{inFlight ? "Starting..." : "Start Reproject"}
			</Button>
		</div>
	</div>

	{#if showRange}
		<div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
			<div class="flex flex-col gap-1">
				<label class="text-xs font-medium" style="color: var(--text-secondary);">
					Range Start (RFC3339)
				</label>
				<input
					type="text"
					class="rounded-md border px-3 py-1.5 text-sm font-mono"
					style="background: var(--surface-bg); border-color: var(--surface-border, #d1d5db); color: var(--text-primary);"
					placeholder="2026-03-01T00:00:00Z"
					bind:value={rangeStart}
					disabled={inFlight}
				/>
			</div>
			<div class="flex flex-col gap-1">
				<label class="text-xs font-medium" style="color: var(--text-secondary);">
					Range End (RFC3339)
				</label>
				<input
					type="text"
					class="rounded-md border px-3 py-1.5 text-sm font-mono"
					style="background: var(--surface-bg); border-color: var(--surface-border, #d1d5db); color: var(--text-primary);"
					placeholder="2026-03-18T00:00:00Z"
					bind:value={rangeEnd}
					disabled={inFlight}
				/>
			</div>
		</div>
	{/if}
</div>
