export interface Article {
  id: string;
  title: string;
  content: string;
  url: string;
  published_at: string;
  tags?: string[];
}
