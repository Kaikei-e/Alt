import type { ApiClient } from "../core/ApiClient";
import { ApiError } from "../core/ApiError";
import type { Article } from "@/schema/article";
import type { MorningUpdate } from "@/schema/morning";

type MorningUpdateResponse = {
  group_id: string;
  primary_article: ArticleResponse | null;
  duplicates: ArticleResponse[];
};

type ArticleResponse = {
  id: string;
  feed_id: string;
  tenant_id: string;
  title: string;
  content: string;
  summary: string | null;
  url: string;
  author: string | null;
  language: string | null;
  tags: string[] | null;
  published_at: string;
  created_at: string;
  updated_at: string;
};

const adaptArticle = (article: ArticleResponse): Article => ({
  id: article.id,
  title: article.title,
  content: article.content,
  url: article.url,
  published_at: article.published_at,
  tags: article.tags ?? undefined,
});

const adaptMorningUpdate = (payload: MorningUpdateResponse): MorningUpdate => ({
  group_id: payload.group_id,
  primary_article: payload.primary_article
    ? adaptArticle(payload.primary_article)
    : null,
  duplicates: (payload.duplicates ?? []).map(adaptArticle),
});

export class MorningApi {
  constructor(private apiClient: ApiClient) { }

  async getOvernightUpdates(): Promise<MorningUpdate[]> {
    try {
      const response = await this.apiClient.get<MorningUpdateResponse[]>(
        "/v1/morning-letter/updates",
        10, // 10秒キャッシュ
      );
      return (response ?? []).map(adaptMorningUpdate);
    } catch (error) {
      throw new ApiError(
        "Failed to fetch morning updates",
        500,
        error as string,
      );
    }
  }
}

