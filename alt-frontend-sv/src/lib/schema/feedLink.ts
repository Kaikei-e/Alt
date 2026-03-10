export type FeedHealthStatus = "healthy" | "warning" | "error" | "inactive" | "unknown";

export interface FeedLink {
	id: string;
	url: string;
	healthStatus: FeedHealthStatus;
	consecutiveFailures: number;
	lastFailureReason: string;
	isActive: boolean;
}
