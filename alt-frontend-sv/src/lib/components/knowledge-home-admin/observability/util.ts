/**
 * Pure helpers for the Observability panel.
 *
 * computeDelta: trailing-half vs leading-half mean, used for MetricRow Δ.
 * topSeries:    order per-series digest rows by lead value, cap to N.
 *
 * Kept framework-free so the logic can be unit-tested without Svelte runes.
 */

export type Direction = "up" | "down" | "flat";

export interface Delta {
	absolute: number;
	percent: number;
	direction: Direction;
}

export interface SimplePoint {
	time: string;
	value: number;
}

export interface SimpleSeries {
	labels: Record<string, string>;
	points: SimplePoint[];
}

export interface TopSeriesRow {
	labelValue: string;
	lead: number;
	original: SimpleSeries;
}

export interface TopSeriesResult {
	head: TopSeriesRow[];
	overflow: number;
}

// Changes smaller than this (relative to the baseline) render as "flat".
// Keeps the eye from being drawn to sub-1% noise on a sparkline view.
const FLAT_RATIO_THRESHOLD = 0.005;

export function computeDelta(points: SimplePoint[]): Delta | null {
	if (points.length < 4) {
		return null;
	}
	const mid = Math.floor(points.length / 2);
	const leadAvg = average(points.slice(0, mid).map((p) => p.value));
	const trailAvg = average(points.slice(mid).map((p) => p.value));
	const absolute = trailAvg - leadAvg;
	let percent: number;
	if (leadAvg === 0) {
		// Baseline is zero: percent is meaningless. Use trailAvg directly as a
		// dimensionless signal so the caller can render "—" or a plain sign.
		percent = trailAvg === 0 ? 0 : trailAvg > 0 ? 100 : -100;
	} else {
		percent = (absolute / Math.abs(leadAvg)) * 100;
	}
	const direction: Direction =
		Math.abs(absolute) < Math.abs(leadAvg) * FLAT_RATIO_THRESHOLD
			? "flat"
			: absolute > 0
				? "up"
				: "down";
	return { absolute, percent, direction };
}

export function topSeries(
	series: SimpleSeries[],
	preferLabel: string,
	limit: number,
): TopSeriesResult {
	if (!series.length) {
		return { head: [], overflow: 0 };
	}
	const withLead = series
		.map<TopSeriesRow>((s) => {
			const lead = s.points.at(-1)?.value ?? 0;
			return {
				labelValue: deriveLabel(s.labels, preferLabel),
				lead,
				original: s,
			};
		})
		.sort((a, b) => b.lead - a.lead);
	const head = withLead.slice(0, Math.max(0, limit));
	const overflow = Math.max(0, withLead.length - head.length);
	return { head, overflow };
}

function average(nums: number[]): number {
	if (!nums.length) return 0;
	let sum = 0;
	for (const n of nums) sum += n;
	return sum / nums.length;
}

function deriveLabel(
	labels: Record<string, string>,
	preferred: string,
): string {
	const direct = labels[preferred];
	if (direct) return direct;
	// Fallback: compact `k=v` pairs, excluding Prometheus bookkeeping labels.
	const parts: string[] = [];
	for (const [k, v] of Object.entries(labels)) {
		if (k === "__name__" || k === "instance" || k === "service") continue;
		parts.push(`${k}=${v}`);
	}
	return parts.join(" ") || "(default)";
}
