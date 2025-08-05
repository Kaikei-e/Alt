export interface FeedStatsSummary {
  feed_amount: {
    amount: number;
  };
  summarized_feed: {
    amount: number;
  };
}

export interface UnsummarizedFeedStatsSummary {
  feed_amount: {
    amount: number;
  };
  unsummarized_feed: {
    amount: number;
  };
  total_articles: {
    amount: number;
  };
}
