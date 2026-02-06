import type { PipelineStage, JobStatus, GenreStatusType } from "./types";

export function getStageLabel(stage: PipelineStage): string {
	const labels: Record<PipelineStage, string> = {
		fetch: "Fetch",
		preprocess: "Preprocess",
		dedup: "Dedup",
		genre: "Genre",
		select: "Select",
		evidence: "Evidence",
		dispatch: "Dispatch",
		persist: "Persist",
	};
	return labels[stage];
}

export function getStatusColor(status: JobStatus | GenreStatusType): string {
	const colors: Record<JobStatus | GenreStatusType, string> = {
		pending: "text-gray-500",
		running: "text-blue-500",
		completed: "text-green-500",
		succeeded: "text-green-500",
		failed: "text-red-500",
	};
	return colors[status] ?? "text-gray-500";
}

export function getStatusBgColor(status: JobStatus | GenreStatusType): string {
	const colors: Record<JobStatus | GenreStatusType, string> = {
		pending: "bg-gray-100",
		running: "bg-blue-100",
		completed: "bg-green-100",
		succeeded: "bg-green-100",
		failed: "bg-red-100",
	};
	return colors[status] ?? "bg-gray-100";
}

export function formatDuration(seconds: number | null): string {
	if (seconds === null) return "-";
	if (seconds < 60) return `${seconds}s`;
	if (seconds < 3600) return `${Math.floor(seconds / 60)}m ${seconds % 60}s`;
	const hours = Math.floor(seconds / 3600);
	const minutes = Math.floor((seconds % 3600) / 60);
	return `${hours}h ${minutes}m`;
}
