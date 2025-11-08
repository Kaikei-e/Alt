import type { ApiClient } from "../core/ApiClient";
import { ApiError } from "../core/ApiError";
import type { EvidenceLink, RecapGenre, RecapSummary } from "@/schema/recap";

type RecapSummaryResponse = {
  job_id: string;
  executed_at: string;
  window_start: string;
  window_end: string;
  total_articles: number;
  genres: RecapGenreResponse[];
};

type RecapGenreResponse = {
  genre: string;
  summary: string;
  top_terms?: string[] | null;
  article_count: number;
  cluster_count: number;
  evidence_links?: EvidenceLinkResponse[] | null;
};

type EvidenceLinkResponse = {
  article_id: string;
  title: string;
  source_url: string;
  published_at: string;
  lang: string;
};

const adaptEvidenceLink = (link: EvidenceLinkResponse): EvidenceLink => ({
  articleId: link.article_id,
  title: link.title,
  sourceUrl: link.source_url,
  publishedAt: link.published_at,
  lang: link.lang,
});

const adaptGenre = (genre: RecapGenreResponse): RecapGenre => ({
  genre: genre.genre,
  summary: genre.summary,
  topTerms: genre.top_terms ?? [],
  articleCount: genre.article_count,
  clusterCount: genre.cluster_count,
  evidenceLinks: (genre.evidence_links ?? []).map(adaptEvidenceLink),
});

const adaptRecapSummary = (payload: RecapSummaryResponse): RecapSummary => ({
  jobId: payload.job_id,
  executedAt: payload.executed_at,
  windowStart: payload.window_start,
  windowEnd: payload.window_end,
  totalArticles: payload.total_articles,
  genres: (payload.genres ?? []).map(adaptGenre),
});

export class RecapApi {
  constructor(private apiClient: ApiClient) {}

  async get7DaysRecap(): Promise<RecapSummary> {
    try {
      const response = await this.apiClient.get<RecapSummaryResponse>(
        "/v1/recap/7days",
        30 // 30秒タイムアウト（バックエンドがrecap-workerから取得するため長め）
      );
      return adaptRecapSummary(response);
    } catch (error) {
      throw new ApiError("Failed to fetch 7-day recap", 500, error as string);
    }
  }
}

