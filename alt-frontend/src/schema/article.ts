export interface Article {
  id: string;
  title: string;
  content: string;
}

// Backend response uses capitalized field names
export interface ArticleBackendResponse {
  ID: string;
  Title: string;
  Content: string;
}
