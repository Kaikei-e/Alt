export type Feed = {
  id: string;
  title: string;
  description: string;
  link: string;
  published: string;
};

export interface BackendFeedItem {
  title: string;
  description: string;
  link: string;
  links?: string[];
  published?: string;
  author?: {
    name: string;
  };
  authors?: Array<{
    name: string;
  }>;
}

export interface FeedURLPayload {
  feed_url: string;
}

export interface FeedDetails {
  feed_url: string;
  summary: string;
}
