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

export function localTimeZone(): string {
	return Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
}

export function formatLocalTime(input: string | number | Date): string {
	let ms: number;
	if (input instanceof Date) {
		ms = input.getTime();
	} else if (typeof input === "number") {
		if (!Number.isFinite(input)) return `${input}`;
		ms = input;
	} else {
		ms = Date.parse(input);
		if (!Number.isFinite(ms)) return input;
	}
	const d = new Date(ms);
	const hh = String(d.getHours()).padStart(2, "0");
	const mm = String(d.getMinutes()).padStart(2, "0");
	const ss = String(d.getSeconds()).padStart(2, "0");
	return `${hh}:${mm}:${ss}`;
}
