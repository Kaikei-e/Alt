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
};
