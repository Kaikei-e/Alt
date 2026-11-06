<script lang="ts">
import { badge, type CertRow, THRESHOLDS } from "./model";

let {
	rows,
	availability,
}: {
	rows: CertRow[];
	availability: { available: boolean; reason: string | null };
} = $props();

function formatRemaining(sec: number | null): string {
	if (sec === null || !Number.isFinite(sec)) return "NO DATA";
	const s = Math.max(0, Math.round(sec));
	if (s < 60) return `${s}s`;
	if (s < 3600) {
		const m = Math.floor(s / 60);
		return `${m}m (${s}s)`;
	}
	const h = Math.floor(s / 3600);
	const m = Math.floor((s % 3600) / 60);
	return m > 0 ? `${h}h ${m}m` : `${h}h`;
}
</script>

<section id="certificates" class="certificates" aria-label="Certificates">
	<header class="section-bar">
		<h2 class="section-head">Certificates — mTLS Leaf Enrollment</h2>
		<span class="threshold-meta">
			remaining {THRESHOLDS.certRemaining.display} · unhealthy enrollment = CRIT
		</span>
	</header>

	<div class="table-wrap">
		{#if !availability.available}
			<p class="dim">
				NO DATA — certificate enrollment unavailable{availability.reason ? ` (${availability.reason})` : ""}
			</p>
		{:else if rows.length === 0}
			<p class="empty">No certificate enrollment metrics reported.</p>
		{:else}
			<table>
				<thead>
					<tr>
						<th scope="col">State</th>
						<th scope="col">Job</th>
						<th scope="col">Enrollment</th>
						<th scope="col" class="num-col">Remaining</th>
					</tr>
				</thead>
				<tbody>
					{#each rows as r (r.job)}
						{@const b = badge(r.severity)}
						<tr data-severity={b.severity}>
							<td>
								<span class="state-badge" data-severity={b.severity}>
									<span class="glyph" aria-hidden="true">{b.glyph}</span>
									<span>{b.label}</span>
								</span>
							</td>
							<td class="job">{r.job}</td>
							<td class="health">
								{r.healthy === null
									? "NO DATA"
									: r.healthy
										? "healthy"
										: "unhealthy"}
							</td>
							<td class="num">{formatRemaining(r.remainingSeconds)}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		{/if}
	</div>
</section>

<style>
	.certificates {
		display: grid;
		gap: 0.5rem;
		min-width: 0;
	}

	.section-bar {
		display: flex;
		justify-content: space-between;
		align-items: baseline;
		flex-wrap: wrap;
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

	.threshold-meta {
		font-family: var(--font-mono);
		font-size: 0.68rem;
		color: var(--alt-slate);
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

	.state-badge {
		display: inline-flex;
		align-items: baseline;
		gap: 0.3rem;
		font-family: var(--font-mono);
		font-size: 0.72rem;
		font-weight: 600;
		letter-spacing: 0.08em;
	}

	.state-badge[data-severity="ok"] {
		color: var(--obs-good, var(--alt-success));
	}

	.state-badge[data-severity="warn"] {
		color: var(--obs-warn, var(--alt-warning));
	}

	.state-badge[data-severity="crit"] {
		color: var(--obs-critical, var(--alt-error));
	}

	.job,
	.health,
	.num {
		font-family: var(--font-mono);
		font-size: 0.78rem;
	}

	.num {
		text-align: right;
		white-space: nowrap;
	}

	.empty {
		color: var(--alt-ash);
		font-style: italic;
		margin: 0.4rem 0;
	}

	.dim {
		margin: 0.4rem 0;
		font-family: var(--font-mono);
		font-size: 0.74rem;
		color: var(--alt-ash);
	}
</style>
