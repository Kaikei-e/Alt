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
