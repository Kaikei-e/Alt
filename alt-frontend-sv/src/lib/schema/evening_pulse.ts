/**
 * Evening Pulse type definitions
 */

export type PulseStatus = "normal" | "partial" | "quiet_day" | "error";

export type TopicRole = "need_to_know" | "trend" | "serendipity";

export type Confidence = "high" | "medium" | "low";

export type PulseRationale = {
	text: string;
	confidence: Confidence;
};

export type RepresentativeArticle = {
	articleId: string;
	title: string;
	sourceUrl: string;
	sourceName: string;
	publishedAt: string;
};

export type PulseTopic = {
	clusterId: number;
	role: TopicRole;
	title: string;
	rationale: PulseRationale;
	articleCount: number;
	sourceCount: number;
	tier1Count?: number;
	timeAgo: string;
	trendMultiplier?: number;
	genre?: string;
	articleIds: string[];
	representativeArticles: RepresentativeArticle[];
	topEntities: string[];
	sourceNames: string[];
};

export type WeeklyHighlight = {
	id: string;
	title: string;
	date: string;
	role: string;
};

export type QuietDayInfo = {
	message: string;
	weeklyHighlights: WeeklyHighlight[];
};

export type EveningPulse = {
	jobId: string;
	date: string;
	generatedAt: string;
	status: PulseStatus;
	topics: PulseTopic[];
	quietDay?: QuietDayInfo;
};
