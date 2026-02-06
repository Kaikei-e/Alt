export type FeedAmount = {
	amount: number;
};

export type SummarizedFeedAmount = {
	amount: number;
};

export type FeedStatsSummary = {
	feed_amount: FeedAmount;
	summarized_feed: SummarizedFeedAmount;
};

export type DetailedFeedStatsSummary = {
	feed_amount: FeedAmount;
	total_articles: { amount: number };
	unsummarized_articles: { amount: number };
};

export type UnreadCountResponse = {
	count: number;
};

export type TrendDataPoint = {
	timestamp: string;
	articles: number;
	summarized: number;
	feed_activity: number;
};

export type TrendDataResponse = {
	data_points: TrendDataPoint[];
	granularity: "hourly" | "daily";
	window: string;
};

export type TimeWindow = "4h" | "24h" | "3d" | "7d";
