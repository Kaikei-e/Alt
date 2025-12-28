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
