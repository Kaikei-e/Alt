<script lang="ts">
import { Cpu, HardDrive, Activity, Server, BarChart3 } from "@lucide/svelte";
import { onMount } from "svelte";

interface SystemStats {
	memory: {
		used: number;
		total: number;
		percent: number;
	};
	cpu: {
		percent: number;
	};
	gpu?: {
		available: boolean;
		gpus?: Array<{
			name: string;
			utilization: number;
			memory_percent: number;
			temperature: number;
		}>;
	};
	hanging_count: number;
	top_processes: Array<{
		pid: number;
		name: string;
		cpu_percent: number;
		memory_mb: number;
	}>;
}

let stats = $state<SystemStats | null>(null);
let isConnected = $state(false);
let retryCount = $state(0);
let error = $state<string | null>(null);
let evtSource: EventSource | null = null;

onMount(() => {
	// Connect to SSE endpoint
	const sseUrl = "/sse/dashboard/stream";
	evtSource = new EventSource(sseUrl);

	evtSource.onopen = () => {
		isConnected = true;
		error = null;
		retryCount = 0;
	};

	evtSource.onmessage = (event) => {
		try {
			const data = JSON.parse(event.data);
			stats = data;
		} catch (e) {
			console.error("Failed to parse SSE data:", e);
		}
	};

	evtSource.onerror = () => {
		isConnected = false;
		retryCount++;
		error = "Connection error";
	};

	return () => {
		if (evtSource) {
			evtSource.close();
		}
	};
});
</script>

<div>
	<h2 class="text-2xl font-bold mb-4" style="color: var(--text-primary);">
		System Monitor (Real-time)
	</h2>

	<!-- Connection Status -->
	<div
		class="p-4 border mb-6"
		style="
			background: var(--surface-bg);
			border-color: var(--surface-border);
			box-shadow: var(--shadow-sm);
		"
	>
		<div class="flex items-center gap-2">
			<div
				class="w-2 h-2 transition-colors"
				style="
					background-color: {isConnected
						? 'var(--alt-success)'
						: retryCount > 0
							? 'var(--alt-warning)'
							: 'var(--alt-error)'};
				"
			></div>
			<p class="text-sm" style="color: var(--text-primary);">
				{isConnected
					? "Connected"
					: retryCount > 0
						? `Reconnecting (${retryCount}/5)`
						: "Disconnected"}
			</p>
		</div>
	</div>

	{#if stats}
		<div class="grid grid-cols-1 md:grid-cols-2 gap-6 mb-8">
			<!-- CPU Usage -->
			<div
				class="p-6 border"
				style="
					background: var(--surface-bg);
					border-color: var(--surface-border);
					box-shadow: var(--shadow-sm);
				"
			>
				<div class="flex items-center gap-3 mb-4">
					<Cpu size={24} style="color: var(--alt-primary);" />
					<h3
						class="text-sm font-semibold uppercase tracking-wider"
						style="color: var(--text-primary);"
					>
						CPU Usage
					</h3>
				</div>
				<p
					class="text-3xl font-bold mb-2"
					style="color: var(--text-primary);"
				>
					{stats.cpu.percent.toFixed(1)}%
				</p>
				<div
					class="w-full h-2 mb-2"
					style="background: var(--surface-border);"
				>
					<div
						style="
							width: {stats.cpu.percent}%;
							height: 100%;
							background: var(--alt-primary);
							transition: width 0.3s;
						"
					></div>
				</div>
			</div>

			<!-- Memory Usage -->
			<div
				class="p-6 border"
				style="
					background: var(--surface-bg);
					border-color: var(--surface-border);
					box-shadow: var(--shadow-sm);
				"
			>
				<div class="flex items-center gap-3 mb-4">
					<HardDrive size={24} style="color: var(--alt-primary);" />
					<h3
						class="text-sm font-semibold uppercase tracking-wider"
						style="color: var(--text-primary);"
					>
						Memory Usage
					</h3>
				</div>
				<p
					class="text-3xl font-bold mb-2"
					style="color: var(--text-primary);"
				>
					{stats.memory.percent.toFixed(1)}%
				</p>
				<p class="text-sm mb-2" style="color: var(--text-muted);">
					{(stats.memory.used / 1073741824).toFixed(2)} GB /{" "}
					{(stats.memory.total / 1073741824).toFixed(2)} GB
				</p>
				<div
					class="w-full h-2"
					style="background: var(--surface-border);"
				>
					<div
						style="
							width: {stats.memory.percent}%;
							height: 100%;
							background: var(--alt-primary);
							transition: width 0.3s;
						"
					></div>
				</div>
			</div>

			<!-- Hanging Processes -->
			<div
				class="p-6 border"
				style="
					background: var(--surface-bg);
					border-color: var(--surface-border);
					box-shadow: var(--shadow-sm);
				"
			>
				<div class="flex items-center gap-3 mb-4">
					<Activity size={24} style="color: var(--alt-primary);" />
					<h3
						class="text-sm font-semibold uppercase tracking-wider"
						style="color: var(--text-primary);"
					>
						Hanging Processes
					</h3>
				</div>
				<p
					class="text-3xl font-bold mb-2"
					style="color: var(--text-primary);"
				>
					{stats.hanging_count}
				</p>
				<p class="text-sm" style="color: var(--text-muted);">
					spawn_main / fork
				</p>
			</div>

			<!-- System Status -->
			<div
				class="p-6 border"
				style="
					background: var(--surface-bg);
					border-color: var(--surface-border);
					box-shadow: var(--shadow-sm);
				"
			>
				<div class="flex items-center gap-3 mb-4">
					<Server size={24} style="color: var(--alt-primary);" />
					<h3
						class="text-sm font-semibold uppercase tracking-wider"
						style="color: var(--text-primary);"
					>
						Status
					</h3>
				</div>
				<p
					class="text-3xl font-bold mb-2"
					style="color: {isConnected ? 'var(--alt-success)' : 'var(--alt-error)'};"
				>
					{isConnected ? "Online" : "Offline"}
				</p>
				<p class="text-sm" style="color: var(--text-muted);">
					{stats.top_processes.length} processes monitored
				</p>
			</div>
		</div>

		<!-- GPU Information -->
		{#if stats.gpu?.available && stats.gpu.gpus && stats.gpu.gpus.length > 0}
			<div class="mb-8">
				<h3
					class="text-xl font-bold mb-4"
					style="color: var(--text-primary);"
				>
					GPU Information
				</h3>
				<div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
					{#each stats.gpu.gpus as gpu}
						<div
							class="p-6 border"
							style="
								background: var(--surface-bg);
								border-color: var(--surface-border);
								box-shadow: var(--shadow-sm);
							"
						>
							<h4
								class="text-lg font-semibold mb-4"
								style="color: var(--text-primary);"
							>
								{gpu.name}
							</h4>
							<div class="space-y-4">
								<div>
									<div class="flex justify-between text-sm mb-1">
										<span style="color: var(--text-muted);">
											Utilization
										</span>
										<span
											class="font-semibold"
											style="color: var(--text-primary);"
										>
											{gpu.utilization}%
										</span>
									</div>
									<div
										class="w-full h-2"
										style="background: var(--surface-border);"
									>
										<div
											style="
												width: {gpu.utilization}%;
												height: 100%;
												background: var(--alt-primary);
												transition: width 0.3s;
											"
										></div>
									</div>
								</div>
								<div>
									<div class="flex justify-between text-sm mb-1">
										<span style="color: var(--text-muted);">
											Memory
										</span>
										<span
											class="font-semibold"
											style="color: var(--text-primary);"
										>
											{gpu.memory_percent}%
										</span>
									</div>
									<div
										class="w-full h-2"
										style="background: var(--surface-border);"
									>
										<div
											style="
												width: {gpu.memory_percent}%;
												height: 100%;
												background: var(--alt-secondary);
												transition: width 0.3s;
											"
										></div>
									</div>
								</div>
								<div class="text-sm" style="color: var(--text-muted);">
									Temperature: {gpu.temperature}°C
								</div>
							</div>
						</div>
					{/each}
				</div>
			</div>
		{/if}

		<!-- Top Processes -->
		<div>
			<h3
				class="text-xl font-bold mb-4"
				style="color: var(--text-primary);"
			>
				Top Processes
			</h3>
			<div
				class="border overflow-hidden"
				style="
					background: var(--surface-bg);
					border-color: var(--surface-border);
					box-shadow: var(--shadow-sm);
				"
			>
				<table class="w-full">
					<thead
						style="
							background: var(--surface-hover);
							border-bottom: 1px solid var(--surface-border);
						"
					>
						<tr>
							<th
								class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider"
								style="color: var(--text-muted);"
							>
								PID
							</th>
							<th
								class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider"
								style="color: var(--text-muted);"
							>
								Name
							</th>
							<th
								class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider"
								style="color: var(--text-muted);"
							>
								CPU %
							</th>
							<th
								class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider"
								style="color: var(--text-muted);"
							>
								Memory (MB)
							</th>
						</tr>
					</thead>
					<tbody style="border-top: 1px solid var(--surface-border);">
						{#each stats.top_processes.slice(0, 10) as process}
							<tr
								style="
									border-bottom: 1px solid var(--surface-border);
									transition: background 0.2s;
								"
								onmouseenter={(e) => {
									e.currentTarget.style.background = "var(--surface-hover)";
								}}
								onmouseleave={(e) => {
									e.currentTarget.style.background = "transparent";
								}}
							>
								<td
									class="px-6 py-4 whitespace-nowrap text-sm"
									style="color: var(--text-primary);"
								>
									{process.pid}
								</td>
								<td
									class="px-6 py-4 whitespace-nowrap text-sm"
									style="color: var(--text-primary);"
								>
									{process.name.length > 40
										? process.name.substring(0, 40) + "..."
										: process.name}
								</td>
								<td
									class="px-6 py-4 whitespace-nowrap text-sm"
									style="color: var(--text-primary);"
								>
									{process.cpu_percent.toFixed(1)}%
								</td>
								<td
									class="px-6 py-4 whitespace-nowrap text-sm"
									style="color: var(--text-primary);"
								>
									{process.memory_mb.toFixed(1)}
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		</div>
	{:else}
		<!-- Loading State -->
		<div
			class="p-12 text-center border"
			style="
				background: var(--surface-bg);
				border-color: var(--surface-border);
				box-shadow: var(--shadow-sm);
			"
		>
			<BarChart3
				class="w-12 h-12 mx-auto mb-4"
				style="color: var(--text-muted);"
			/>
			<p style="color: var(--text-muted);">
				{error ? `Error: ${error}` : "Loading system statistics..."}
			</p>
		</div>
	{/if}
</div>

