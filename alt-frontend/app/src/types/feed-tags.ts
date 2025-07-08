export interface FeedTags {
  tags: FeedTag[];
  feed_url: string;
  next_cursor?: string;
}

export interface FeedTag {
  id: number;
  name: string;
  created_at: string;
}