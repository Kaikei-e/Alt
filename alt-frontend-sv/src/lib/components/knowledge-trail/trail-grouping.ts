import type { FootprintData } from "$lib/connect/knowledge_trail";

export interface TrailDayGroup {
	/** Stable key for the day (YYYY-MM-DD in local time). */
	dayKey: string;
	/** Human label, e.g. "Today", "Yesterday", or "June 9, 2026". */
	label: string;
	footprints: FootprintData[];
}

function localDayKey(d: Date): string {
	const y = d.getFullYear();
	const m = String(d.getMonth() + 1).padStart(2, "0");
	const day = String(d.getDate()).padStart(2, "0");
	return `${y}-${m}-${day}`;
}

/**
 * Groups footprints into reverse-chronological day buckets. `now` is injected
 * so the Today/Yesterday labels are deterministic and testable.
 */
export function groupFootprintsByDay(
	footprints: FootprintData[],
	now: Date,
): TrailDayGroup[] {
	const todayKey = localDayKey(now);
	const yesterday = new Date(now);
	yesterday.setDate(now.getDate() - 1);
	const yesterdayKey = localDayKey(yesterday);

	const groups: TrailDayGroup[] = [];
	const byKey = new Map<string, TrailDayGroup>();

	for (const fp of footprints) {
		const d = new Date(fp.occurredAt);
		const key = Number.isNaN(d.getTime()) ? "unknown" : localDayKey(d);
		let group = byKey.get(key);
		if (!group) {
			group = { dayKey: key, label: labelFor(key, d, todayKey, yesterdayKey), footprints: [] };
			byKey.set(key, group);
			groups.push(group);
		}
		group.footprints.push(fp);
	}
	return groups;
}

function labelFor(
	key: string,
	d: Date,
	todayKey: string,
	yesterdayKey: string,
): string {
	if (key === todayKey) return "Today";
	if (key === yesterdayKey) return "Yesterday";
	if (key === "unknown") return "Earlier";
	return d.toLocaleDateString([], {
		year: "numeric",
		month: "long",
		day: "numeric",
	});
}
