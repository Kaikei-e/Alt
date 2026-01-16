<script lang="ts">
	import type { StatusTransition, JobStatus } from "$lib/schema/dashboard";

	interface Props {
		transitions: StatusTransition[];
	}

	let { transitions }: Props = $props();

	function formatTime(isoString: string): string {
		return new Date(isoString).toLocaleTimeString("ja-JP", {
			hour: "2-digit",
			minute: "2-digit",
			second: "2-digit",
		});
	}

	const statusColors: Record<JobStatus, { text: string; bg: string }> = {
		completed: { text: "text-green-600", bg: "bg-green-500" },
		failed: { text: "text-red-600", bg: "bg-red-500" },
		running: { text: "text-blue-600", bg: "bg-blue-500" },
		pending: { text: "text-gray-500", bg: "bg-gray-400" },
	};
</script>

<div class="py-2">
	{#if transitions.length === 0}
		<p class="text-sm" style="color: var(--text-muted);">No status history available.</p>
	{:else}
		<div class="relative pl-4">
			<!-- Timeline line -->
			<div
				class="absolute left-1.5 top-2 bottom-2 w-0.5 bg-gray-200"
			></div>

			{#each transitions as transition, index}
				{@const colors = statusColors[transition.status] ?? statusColors.pending}
				{@const isLast = index === transitions.length - 1}
				<div class="relative flex items-start gap-3 pb-3 {isLast ? 'pb-0' : ''}">
					<!-- Timeline dot -->
					<div
						class="absolute -left-2.5 mt-1 w-3 h-3 rounded-full border-2 border-white {colors.bg}"
					></div>

					<!-- Content -->
					<div class="flex-1 min-w-0">
						<div class="flex items-center gap-2 flex-wrap">
							<span
								class="text-xs font-mono"
								style="color: var(--text-muted);"
							>
								{formatTime(transition.transitioned_at)}
							</span>
							<span class="text-sm font-medium {colors.text}">
								{transition.status}
							</span>
							{#if transition.stage}
								<span
									class="text-xs px-1.5 py-0.5 rounded bg-gray-100"
									style="color: var(--text-muted);"
								>
									@ {transition.stage}
								</span>
							{/if}
						</div>
						{#if transition.reason}
							<p
								class="mt-1 text-xs truncate max-w-md"
								style="color: var(--text-muted);"
								title={transition.reason}
							>
								{transition.reason}
							</p>
						{/if}
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>
