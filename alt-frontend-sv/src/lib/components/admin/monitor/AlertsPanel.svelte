<script lang="ts">
import { formatLocalTime } from "./format";
import { type AlertRow, alertRowSeverity, badge } from "./model";

let {
	rows,
	now = Date.now(),
	availability,
}: {
	rows: AlertRow[];
	now?: number;
	availability: { available: boolean; reason: string | null };
} = $props();

const activeRows = $derived(rows.filter((r) => !r.isHeartbeat));
const heartbeatRows = $derived(rows.filter((r) => r.isHeartbeat));

function formatTarget(labels: Record<string, string>): string {
	const parts: string[] = [];
	for (const key of ["job", "service", "instance", "device", "queue"]) {
		const val = labels[key];
		if (val) parts.push(val);
	}
	return parts.length > 0 ? parts.join(" · ") : "—";
}

function formatDurationShort(sec: number): string {
	const s = Math.max(0, Math.round(sec));
	if (s < 60) return `${s}s`;
	if (s < 3600) {
		const m = Math.floor(s / 60);
		const remS = s % 60;
		return remS > 0 ? `${m}m ${remS}s` : `${m}m`;
	}
	const h = Math.floor(s / 3600);
	const remM = Math.floor((s % 3600) / 60);
	return remM > 0 ? `${h}h ${remM}m` : `${h}h`;
}

function formatActiveSince(activeSince: number | null, nowMs: number): string {
	if (activeSince === null || !Number.isFinite(activeSince)) return "—";
	const sinceMs = activeSince * 1000;
	const ageSec = Math.max(0, Math.round((nowMs - sinceMs) / 1000));
	return `${formatDurationShort(ageSec)} ago (${formatLocalTime(sinceMs)})`;
}

const heartbeatLine = $derived.by(() => {
	if (heartbeatRows.length === 0) return null;
	const names = [...new Set(heartbeatRows.map((r) => r.alertname))].join(", ");
	const state = heartbeatRows[0]?.state ?? "firing";
	return `${names} heartbeat ${state} — expected`;
});
</script>

<section id="alerts" class="alerts" aria-label="Alerts">
	<header class="section-bar">
		<h2 class="section-head">Alerts</h2>
		<span class="count">
			{#if !availability.available}
				alerts unknown
			{:else}
				{activeRows.length} {activeRows.length === 1 ? "alert" : "alerts"} active
			{/if}
		</span>
	</header>

	<div class="table-wrap">
		<table>
			<thead>
				<tr>
					<th scope="col">State</th>
					<th scope="col">Severity</th>
					<th scope="col">Alert</th>
					<th scope="col">Target</th>
					<th scope="col" class="num-col">Active since</th>
				</tr>
			</thead>
			<tbody>
				{#if !availability.available}
					<tr>
						<td colspan="5" class="empty">
							NO DATA — alert state unavailable{availability.reason ? ` (${availability.reason})` : ""}
						</td>
					</tr>
				{:else if activeRows.length === 0}
					<tr>
						<td colspan="5" class="empty">
							No firing or pending alerts.
						</td>
					</tr>
				{:else}
					{#each activeRows as r (r.key)}
						{@const sev = alertRowSeverity(r)}
						{@const b = badge(sev)}
						<tr data-severity={sev}>
							<td class="state-cell">
								<span class="state-pill" data-severity={sev}>
									<span class="glyph" aria-hidden="true">{b.glyph}</span>
									<span class="state-text">{r.state}</span>
								</span>
							</td>
							<td class="sev-cell">
								<span class="sev-text" data-severity={sev}>
									{b.label} · {r.severity}
								</span>
							</td>
							<td class="alert-name">{r.alertname}</td>
							<td class="target">{formatTarget(r.labels)}</td>
							<td class="num">{formatActiveSince(r.activeSince, now)}</td>
						</tr>
					{/each}
				{/if}
			</tbody>
		</table>
	</div>

	{#if heartbeatLine}
		<p class="heartbeat" data-testid="alerts-heartbeat">
			{heartbeatLine}
		</p>
	{/if}
</section>

<style>
	.alerts {
		display: grid;
		gap: 0.5rem;
		min-width: 0;
	}

	.section-bar {
		display: flex;
		justify-content: space-between;
		align-items: baseline;
		gap: 0.5rem;
		border-top: 1px solid var(--obs-rule-strong, var(--surface-border));
		padding-top: 0.55rem;
	}

	.section-head {
		font-family: var(--font-display, var(--font-serif));
		font-size: 0.92rem;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		margin: 0;
		color: var(--alt-charcoal);
	}

	.count {
		font-family: var(--font-mono);
		font-size: 0.72rem;
		color: var(--alt-slate);
		font-variant-numeric: tabular-nums;
	}

	.table-wrap {
		width: 100%;
		max-width: 100%;
		overflow-x: auto;
		min-width: 0;
	}

	table {
		width: 100%;
		border-collapse: collapse;
		font-variant-numeric: tabular-nums;
	}

	thead th {
		font-family: var(--font-body);
		font-size: 0.66rem;
		letter-spacing: 0.14em;
		text-transform: uppercase;
		color: var(--alt-slate);
		text-align: left;
		padding: 0.4rem 0.6rem 0.4rem 0;
		border-bottom: 1px solid var(--alt-charcoal);
		white-space: nowrap;
	}

	.num-col {
		text-align: right;
	}

	tbody td {
		padding: 0.5rem 0.6rem 0.5rem 0;
		font-size: 0.82rem;
		color: var(--alt-charcoal);
		border-bottom: 0.5px solid var(--obs-rule, var(--surface-border));
		vertical-align: baseline;
	}

	.state-pill {
		display: inline-flex;
		align-items: baseline;
		gap: 0.3rem;
		font-family: var(--font-mono);
		font-size: 0.72rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.08em;
	}

	.state-pill[data-severity="crit"],
	.sev-text[data-severity="crit"] {
		color: var(--obs-critical, var(--alt-error));
	}

	.state-pill[data-severity="warn"],
	.sev-text[data-severity="warn"] {
		color: var(--obs-warn, var(--alt-warning));
	}

	.sev-text {
		font-family: var(--font-mono);
		font-size: 0.72rem;
	}

	.alert-name {
		font-family: var(--font-mono);
		font-weight: 500;
	}

	.target {
		font-family: var(--font-mono);
		font-size: 0.76rem;
		color: var(--alt-slate);
	}

	.num {
		text-align: right;
		font-family: var(--font-mono);
		font-size: 0.76rem;
		white-space: nowrap;
	}

	.empty {
		color: var(--alt-ash);
		font-style: italic;
	}

	.heartbeat {
		margin: 0;
		font-family: var(--font-mono);
		font-size: 0.72rem;
		color: var(--obs-muted, var(--alt-ash));
	}
</style>
