import type {
	DegradationReason,
	SurfaceStatus,
} from "#lib/domain/recap/types.js";

export type SurfaceNoticeTone = "info" | "warning" | "error";

export interface SurfaceStatusNotice {
	tone: SurfaceNoticeTone;
	text: string;
}

export interface SurfaceStatusEvaluation {
	isCurrent: boolean;
	statusNotice: SurfaceStatusNotice | null;
	degradationNotices: string[];
}

const DEGRADATION_CODE_MESSAGES: Record<string, string> = {
	extractive_genres:
		"Some genres use extractive excerpts instead of AI summaries.",
	few_cards: "Fewer topic cards than usual met the quality threshold.",
	pulse_fallback: "Topics were chosen with relaxed selection criteria.",
	placeholder_output:
		"The AI draft had empty or placeholder sections, so this letter shows extracted excerpts instead.",
	duplicate_bullets:
		"The AI draft repeated points across sections, so this letter shows extracted excerpts instead.",
	low_japanese_ratio:
		"The AI draft was not mostly in Japanese, so this letter shows extracted excerpts instead.",
	stale_recap_input:
		"Built from a 3-day recap older than the freshness window.",
	recap_unavailable: "Built without a 3-day recap.",
	generation_failed: "This edition was produced in degraded mode.",
};

export function formatSurfaceTimestamp(iso: string): string {
	if (!iso) return "";

	const date = new Date(iso);
	if (Number.isNaN(date.getTime())) {
		return "";
	}

	return date.toLocaleString("en-US", {
		month: "short",
		day: "numeric",
		hour: "numeric",
		minute: "2-digit",
	});
}

export function getDegradationNoticeText(reason: DegradationReason): string {
	const detail = reason.detail?.trim() ?? "";
	if (reason.code === "extractive_genres" && detail) {
		return detail.endsWith(".") ? detail : `${detail}.`;
	}
	const base =
		DEGRADATION_CODE_MESSAGES[reason.code] ??
		"Some results were generated in degraded mode.";
	if (!detail) {
		return base;
	}
	return base.endsWith(".")
		? `${base.slice(0, -1)} (${detail}).`
		: `${base} (${detail})`;
}

function buildStatusNotice(
	surfaceStatus: SurfaceStatus,
): SurfaceStatusNotice | null {
	const latestRun = surfaceStatus.latestRun;

	if (latestRun?.status === "failed") {
		const failTime = formatSurfaceTimestamp(
			latestRun.updatedAt || latestRun.kickedAt || "",
		);
		const generatedTime = formatSurfaceTimestamp(surfaceStatus.generatedAt);

		if (failTime && generatedTime) {
			return {
				tone: "error",
				text: `The latest update failed at ${failTime}. Showing results from ${generatedTime}.`,
			};
		}
		if (failTime) {
			return {
				tone: "error",
				text: `The latest update failed at ${failTime}.`,
			};
		}
		if (generatedTime) {
			return {
				tone: "error",
				text: `The latest update failed. Showing results from ${generatedTime}.`,
			};
		}
		return {
			tone: "error",
			text: "The latest update failed.",
		};
	}

	if (surfaceStatus.isStale) {
		const generatedTime = formatSurfaceTimestamp(surfaceStatus.generatedAt);
		return {
			tone: "warning",
			text: generatedTime
				? `This was last updated ${generatedTime}; a newer update has not completed.`
				: "This edition is out of date; a newer update has not completed.",
		};
	}

	if (latestRun?.status === "pending" || latestRun?.status === "running") {
		return {
			tone: "info",
			text: "Update in progress…",
		};
	}

	return null;
}

export function evaluateSurfaceStatus(
	surfaceStatus?: SurfaceStatus | null,
): SurfaceStatusEvaluation {
	if (!surfaceStatus) {
		return {
			isCurrent: false,
			statusNotice: null,
			degradationNotices: [],
		};
	}

	return {
		isCurrent: !surfaceStatus.isStale,
		statusNotice: buildStatusNotice(surfaceStatus),
		degradationNotices: (surfaceStatus.degradationReasons ?? [])
			.map(getDegradationNoticeText)
			.filter((text) => text.length > 0),
	};
}
