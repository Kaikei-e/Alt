<script lang="ts">
import type { PulseRationale as PulseRationaleType } from "$lib/schema/evening_pulse";
import PulseConfidenceIndicator from "./PulseConfidenceIndicator.svelte";

interface Props {
	rationale: PulseRationaleType;
	id?: string;
}

const { rationale, id }: Props = $props();
</script>

<div class="flex flex-col gap-1" {id}>
	<p class="text-sm leading-relaxed" style="color: var(--text-secondary);">
		{rationale.text}
	</p>
	<div class="flex items-center gap-2">
		<PulseConfidenceIndicator confidence={rationale.confidence} />
		<span class="text-xs capitalize" style="color: var(--text-muted);">
			{rationale.confidence} confidence
		</span>
	</div>
</div>
