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
	few_cards:
		"Degraded generation: fewer topic cards than usual met the quality threshold.",
	pulse_fallback: "Some topics were selected using fallback criteria.",
	placeholder_output: "Some sections contain placeholder content.",
	duplicate_bullets: "Some bullet points were repeated across sections.",
	low_japanese_ratio: "Some sections could not be fully translated.",
	stale_recap_input: "Built from an older recap snapshot.",
	recap_unavailable: "Generated without a 3-day recap snapshot.",
	generation_failed: "The latest generation did not complete cleanly.",
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
	const detail = reason.detail?.trim();
	if (detail) {
		return detail;
	}
	return (
		DEGRADATION_CODE_MESSAGES[reason.code] ??
		"Some results were generated in degraded mode."
	);
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

	if (
		(latestRun?.status === "pending" || latestRun?.status === "running") &&
		(!surfaceStatus.isStale || !surfaceStatus.generatedAt)
	) {
		return {
			tone: "info",
			text: "Update in progress…",
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
