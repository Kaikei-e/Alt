<script lang="ts">
let {
	services,
}: {
	services: {
		serviceName: string;
		endpoint: string;
		status: string;
		latencyMs: number;
		checkedAt: string;
		errorMessage: string;
	}[];
} = $props();

const statusBadge = (status: string) => {
	switch (status) {
		case "healthy":
			return { color: "var(--accent-green, #22c55e)", label: "healthy" };
		case "unhealthy":
			return { color: "var(--accent-red, #ef4444)", label: "unhealthy" };
		default:
			return { color: "var(--text-secondary, #6b7280)", label: "unknown" };
	}
};

const formatTime = (iso: string) => {
	if (!iso) return "--";
	try {
		return new Date(iso).toLocaleTimeString("ja-JP");
	} catch {
		return "--";
	}
};
</script>

<div class="flex flex-col gap-4">
	<h3 class="text-sm font-semibold" style="color: var(--text-primary);">
		Service Health
	</h3>

	<div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
		{#each services as service (service.serviceName)}
			{@const badge = statusBadge(service.status)}
			<div
				class="flex flex-col gap-2 rounded-lg border-2 p-4"
				style="background: var(--surface-bg); border-color: var(--border-primary);"
			>
				<div class="flex items-center justify-between">
					<span class="text-sm font-bold" style="color: var(--text-primary);">
						{service.serviceName}
					</span>
					<span
						class="inline-block rounded px-2 py-0.5 text-xs font-medium text-white"
						style="background: {badge.color};"
					>
						{badge.label}
					</span>
				</div>

				<div class="flex items-center gap-4 text-xs" style="color: var(--text-secondary);">
					<span class="font-mono">{service.latencyMs}ms</span>
					<span>{formatTime(service.checkedAt)}</span>
				</div>

				{#if service.status === "unhealthy" && service.errorMessage}
					<div
						class="rounded border px-2 py-1 text-xs"
						style="color: var(--accent-red, #ef4444); border-color: var(--accent-red, #ef4444); background: color-mix(in srgb, var(--accent-red, #ef4444) 8%, transparent);"
					>
						{service.errorMessage}
					</div>
				{/if}
			</div>
		{/each}
	</div>
</div>
