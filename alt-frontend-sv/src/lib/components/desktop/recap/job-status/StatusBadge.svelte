<script lang="ts">
import type { JobStatus, GenreStatusType } from "$lib/schema/dashboard";
import StatusGlyph from "$lib/components/recap/job-status/StatusGlyph.svelte";

interface Props {
	status: JobStatus | GenreStatusType;
	size?: "sm" | "md";
}

let { status, size = "md" }: Props = $props();
const isRunning = $derived(status === "running");
</script>

<span class="status-badge" data-status={status} data-size={size}>
	<StatusGlyph {status} pulse={isRunning} includeLabel={true} />
</span>

<style>
	.status-badge {
		display: inline-flex;
		align-items: baseline;
		gap: 0.4rem;
	}

	.status-badge[data-size="sm"] {
		font-size: 0.65rem;
	}

	.status-badge[data-size="md"] {
		font-size: 0.75rem;
	}
</style>
