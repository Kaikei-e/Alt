/**
 * Formatting helpers for the admin monitor dashboard.
 * Kept framework-free so they can be unit-tested without Svelte.
 */

export function formatValue(
	v: number | null,
	unit: string | undefined,
): string {
	if (v == null || !Number.isFinite(v)) return "—";
	if (unit === "bytes") return formatBytes(v);
	if (unit === "ratio") return `${(v * 100).toFixed(2)}%`;
	if (unit === "seconds") {
		if (Math.abs(v) >= 1) return `${v.toFixed(2)} s`;
		return `${(v * 1000).toFixed(0)} ms`;
	}
	if (unit === "bool") return v >= 1 ? "up" : "down";
	if (Math.abs(v) >= 1000) return v.toFixed(0);
	if (Math.abs(v) >= 1) return v.toFixed(2);
	return v.toFixed(3);
}

export function formatBytes(n: number): string {
	const units = ["B", "KiB", "MiB", "GiB", "TiB"];
	let v = n;
	let i = 0;
	while (v >= 1024 && i < units.length - 1) {
		v /= 1024;
		i += 1;
	}
	return `${v.toFixed(v >= 100 ? 0 : v >= 10 ? 1 : 2)} ${units[i]}`;
}

export type StateGlyph = "▲" | "▼" | "●" | "○";

export interface StateBadge {
	glyph: StateGlyph;
	text: "up" | "down" | "warn" | "ok";
}

/**
 * Maps a metric leading value to a (glyph, text) badge.
 * Text + glyph encoding so meaning never depends on color alone (WCAG 1.4.1).
 *
 * When `warn` evaluates true the badge is a hollow ● + "warn" (above-threshold).
 * `up`/`down` are reserved for the `bool` unit. Otherwise the badge is `ok`.
 */
export function stateBadge(
	value: number | null,
	unit: string | undefined,
	warn?: (v: number) => boolean,
): StateBadge {
	if (value == null || !Number.isFinite(value)) {
		return { glyph: "○", text: "warn" };
	}
	if (unit === "bool") {
		return value >= 1
			? { glyph: "▲", text: "up" }
			: { glyph: "▼", text: "down" };
	}
	if (warn?.(value)) {
		return { glyph: "●", text: "warn" };
	}
	return { glyph: "▲", text: "ok" };
}

/** Burn-rate severity buckets used by SLOBurnPanel. 99.9% baseline. */
export function burnSeverity(
	value: number | null,
): "ok" | "ticket" | "page2" | "page1" {
	if (value == null || !Number.isFinite(value)) return "ok";
	if (value >= 14.4) return "page1";
	if (value >= 6) return "page2";
	if (value >= 1) return "ticket";
	return "ok";
}
