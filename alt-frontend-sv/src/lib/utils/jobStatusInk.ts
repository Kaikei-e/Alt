import type { JobStatus, GenreStatusType } from "$lib/schema/dashboard";

export type StatusInk = "success" | "warning" | "error" | "neutral" | "muted";
export type StatusGlyph = "✓" | "●" | "○" | "✗";

export type StatusInput = JobStatus | GenreStatusType;

export function statusToInk(status: StatusInput): StatusInk {
	switch (status) {
		case "completed":
		case "succeeded":
			return "success";
		case "failed":
			return "error";
		case "running":
			return "neutral";
		case "pending":
			return "muted";
	}
}

export function statusToGlyph(status: StatusInput): StatusGlyph {
	switch (status) {
		case "completed":
		case "succeeded":
			return "✓";
		case "failed":
			return "✗";
		case "running":
			return "●";
		case "pending":
			return "○";
	}
}

export function statusToLabel(status: StatusInput): string {
	switch (status) {
		case "completed":
			return "Completed";
		case "succeeded":
			return "Succeeded";
		case "failed":
			return "Failed";
		case "running":
			return "Running";
		case "pending":
			return "Pending";
	}
}
