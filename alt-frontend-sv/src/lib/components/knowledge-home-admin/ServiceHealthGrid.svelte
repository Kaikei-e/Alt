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

const statusInfo = (status: string): { color: string; status: "ok" | "error" | "neutral" } => {
	switch (status) {
		case "healthy":
			return { color: "var(--alt-sage)", status: "ok" };
		case "unhealthy":
			return { color: "var(--alt-terracotta)", status: "error" };
		default:
			return { color: "var(--alt-ash)", status: "neutral" };
	}
};

const formatTime = (iso: string) => {
	if (!iso) return "--";
	try {
		return new Date(iso).toLocaleTimeString();
	} catch {
		return "--";
	}
};
</script>

<div class="panel" data-role="service-health">
	<h3 class="section-heading">Service Health</h3>
	<div class="heading-rule"></div>

	<div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
		{#each services as service (service.serviceName)}
			{@const info = statusInfo(service.status)}
			<div class="service-card" data-status={info.status}>
				<div class="service-stripe" style="background: {info.color};"></div>
				<div class="service-body">
					<div class="service-header">
						<span class="service-name">{service.serviceName}</span>
						<span class="service-status">{service.status}</span>
					</div>
					<div class="service-meta">
						<span class="service-latency">{service.latencyMs}ms</span>
						<span class="service-time">{formatTime(service.checkedAt)}</span>
					</div>
					{#if service.status === "unhealthy" && service.errorMessage}
						<p class="service-error">{service.errorMessage}</p>
					{/if}
				</div>
			</div>
		{/each}
	</div>
</div>

<style>
	.panel {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
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

	.service-card {
		display: flex;
		border: 1px solid var(--surface-border);
		background: var(--surface-bg);
	}

	.service-stripe {
		width: 3px;
		flex-shrink: 0;
	}

	.service-body {
		display: flex;
		flex-direction: column;
		gap: 0.35rem;
		padding: 0.6rem 0.75rem;
		flex: 1;
	}

	.service-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
	}

	.service-name {
		font-family: var(--font-body);
		font-size: 0.8rem;
		font-weight: 700;
		color: var(--alt-charcoal);
	}

	.service-status {
		font-family: var(--font-mono);
		font-size: 0.6rem;
		font-weight: 600;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--alt-ash);
	}

	.service-meta {
		display: flex;
		align-items: center;
		gap: 0.75rem;
	}

	.service-latency {
		font-family: var(--font-mono);
		font-size: 0.65rem;
		color: var(--alt-charcoal);
	}

	.service-time {
		font-family: var(--font-mono);
		font-size: 0.65rem;
		color: var(--alt-ash);
	}

	.service-error {
		font-family: var(--font-body);
		font-size: 0.7rem;
		color: var(--alt-terracotta);
		margin: 0;
		padding: 0.25rem 0.5rem;
		border-left: 2px solid var(--alt-terracotta);
	}
</style>
