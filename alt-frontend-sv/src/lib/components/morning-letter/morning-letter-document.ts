/**
 * Pure presentation logic for Morning Letter document.
 * No Svelte imports — testable with vitest server project.
 *
 * Uses structural typing compatible with generated proto types
 * (MorningLetterSection, MorningLetterSourceProto, Timestamp).
 */

import type { SurfaceStatus } from "#lib/domain/recap/types.js";

export type { SurfaceStatus };

interface Section {
	key: string;
	title: string;
	bullets: string[];
	genre?: string;
	// Allow additional proto fields via index signature
	[key: string]: unknown;
}

interface Source {
	letterId: string;
	sectionKey: string;
	articleId: string;
	sourceType: number;
	position: number;
	[key: string]: unknown;
}

const SECTION_ORDER: Record<string, number> = {
	top3: 0,
	what_changed: 1,
};

const SECTION_DISPLAY_TITLES: Record<string, string> = {
	top3: "Top Stories",
	what_changed: "What Changed",
};

/**
 * Titles the LLM frequently emits that are too generic to display —
 * we shadow them with a better derived title (mapped / genre / theme).
 */
const GENERIC_TITLE_BLOCKLIST = new Set(
	[
		"Need to Know",
		"Today's Headlines",
		"Top Stories",
		"What Changed",
		"Overview",
		"Summary",
	].map((t) => t.toLowerCase()),
);

export function orderSections<T extends Section>(sections: T[]): T[] {
	return [...sections].sort((a, b) => {
		const orderA = SECTION_ORDER[a.key] ?? 100;
		const orderB = SECTION_ORDER[b.key] ?? 100;
		return orderA - orderB;
	});
}

export function formatLetterDate(
	targetDate: string,
	_editionTimezone?: string,
): string {
	// Decompose civil date string directly — no new Date() to avoid timezone off-by-one
	const parts = targetDate.split("-");
	if (parts.length !== 3) return targetDate;

	const year = parts[0];
	const month = parseInt(parts[1] ?? "0", 10);
	const day = parseInt(parts[2] ?? "0", 10);

	return `${year}-${month}-${day}`;
}

export function formatEditionDateline({
	targetDate,
	editionTimezone,
	isCurrent,
}: {
	targetDate?: string;
	editionTimezone?: string;
	isCurrent: boolean;
}): string {
	if (isCurrent) {
		return "Today's Edition";
	}
	if (targetDate) {
		return formatLetterDate(targetDate, editionTimezone);
	}
	return "Latest Edition";
}

export function getSectionDisplayTitle(
	section: Pick<Section, "key" | "title">,
): string {
	const mapped = SECTION_DISPLAY_TITLES[section.key];

	// by_genre:<genre> and by_theme:<slug> → derive from key.
	let derived = "";
	if (section.key.startsWith("by_genre:")) {
		const genre = section.key.slice("by_genre:".length);
		derived = genre.charAt(0).toUpperCase() + genre.slice(1);
	} else if (section.key.startsWith("by_theme:")) {
		const theme = section.key.slice("by_theme:".length);
		derived = theme
			.split(/[-_\s]+/)
			.filter(Boolean)
			.map((w) => w.charAt(0).toUpperCase() + w.slice(1))
			.join(" ");
	}

	// The LLM often writes generic titles like "Need to Know" across every
	// section. When we have a better derived/mapped title, prefer it.
	const trimmedTitle = section.title?.trim() ?? "";
	const titleIsGeneric =
		trimmedTitle === "" ||
		GENERIC_TITLE_BLOCKLIST.has(trimmedTitle.toLowerCase());

	if (titleIsGeneric) {
		return mapped || derived || section.key;
	}
	return trimmedTitle;
}

export function getSourcesForSection(
	sources: Source[],
	sectionKey: string,
): Source[] {
	return sources.filter((s) => s.sectionKey === sectionKey);
}

export function deriveWithinHours(targetDate: string | undefined): number {
	if (!targetDate) return 24;

	// Parse civil date by decomposition (no new Date() timezone issues)
	const parts = targetDate.split("-");
	if (parts.length !== 3) return 24;

	const year = parseInt(parts[0] ?? "0", 10);
	const month = parseInt(parts[1] ?? "1", 10) - 1; // JS months are 0-based
	const day = parseInt(parts[2] ?? "1", 10);

	// Create date at start of day in local timezone
	const targetStart = new Date(year, month, day, 6, 0, 0); // 6 AM
	const now = Date.now();

	const todayStr = new Date().toISOString().split("T")[0];
	if (targetDate === todayStr) return 24;

	const hoursSinceTarget = (now - targetStart.getTime()) / (1000 * 60 * 60);
	const clamped = Math.max(24, Math.min(168, Math.round(hoursSinceTarget)));

	return clamped;
}
