<script lang="ts">
	import { RefreshCw, Rocket } from "@lucide/svelte";

	interface Props {
		onRefresh: () => void;
		onTriggerJob: () => void;
		loading: boolean;
		triggering: boolean;
		hasRunningJob: boolean;
		justStartedJobId: string | null;
	}

	let {
		onRefresh,
		onTriggerJob,
		loading,
		triggering,
		hasRunningJob,
		justStartedJobId,
	}: Props = $props();

	const isStartDisabled = $derived(triggering || hasRunningJob || justStartedJobId !== null);

	const startButtonTooltip = $derived.by(() => {
		if (justStartedJobId) return "Job is starting...";
		if (hasRunningJob) return "A job is already running";
		return "Start a new recap job";
	});
</script>

<div
	class="fixed bottom-0 left-0 right-0 z-50 border-t bg-white"
	style="border-color: var(--surface-border); padding-bottom: env(safe-area-inset-bottom, 0px);"
	data-testid="mobile-control-bar"
>
	<div class="flex gap-3 p-4">
		<!-- Refresh button -->
		<button
			class="flex-1 flex items-center justify-center gap-2 h-12 rounded-xl border transition-colors disabled:opacity-50"
			style="border-color: var(--surface-border); color: var(--text-primary); background: var(--surface-bg);"
			onclick={onRefresh}
			disabled={loading}
			aria-label="Refresh job data"
		>
			<RefreshCw class="w-5 h-5 {loading ? 'animate-spin' : ''}" />
			<span class="font-medium">Refresh</span>
		</button>

		<!-- Start Job button -->
		<button
			class="flex-1 flex items-center justify-center gap-2 h-12 rounded-xl transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
			style="background: var(--alt-primary, #2f4f4f); color: #ffffff;"
			onclick={onTriggerJob}
			disabled={isStartDisabled}
			title={startButtonTooltip}
			aria-label="Start new recap job"
		>
			<Rocket class="w-5 h-5 {triggering ? 'animate-pulse' : ''}" />
			<span class="font-medium">
				{#if triggering}
					Starting...
				{:else}
					Start Job
				{/if}
			</span>
		</button>
	</div>
</div>
