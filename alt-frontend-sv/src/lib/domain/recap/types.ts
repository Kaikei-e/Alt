export type SurfaceRunStatus = "pending" | "running" | "completed" | "failed";

export type SurfaceRun = {
	jobId: string;
	status: SurfaceRunStatus;
	kickedAt: string;
	updatedAt: string;
};

export type DegradationReason = {
	code: string;
	detail: string;
};

export type SurfaceStatus = {
	generatedAt: string;
	windowStart: string;
	windowEnd: string;
	isStale: boolean;
	staleAfter: string;
	latestRun?: SurfaceRun | null;
	degradationReasons: DegradationReason[];
};

export type RecapGenre = {
	genre: string;
	summary: string;
	topTerms: string[];
	articleCount: number;
	clusterCount: number;
	evidenceLinks: EvidenceLink[];
	bullets: string[];
};

export type EvidenceLink = {
	articleId: string;
	title: string;
	sourceUrl: string;
	publishedAt: string;
	lang: "en" | "ja" | string;
};

export type RecapSummary = {
	jobId: string;
	executedAt: string;
	windowStart: string;
	windowEnd: string;
	totalArticles: number;
	genres: RecapGenre[];
	surfaceStatus?: SurfaceStatus | null;
};
