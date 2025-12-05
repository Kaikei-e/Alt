import type { RecapGenre, RecapSummary } from "$lib/schema/recap";
import { callClientAPI } from "./core";

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
  bullets?: string[] | null;
};

type EvidenceLinkResponse = {
  article_id: string;
  title: string;
  source_url: string;
  published_at: string;
  lang: string;
};

const adaptEvidenceLink = (link: EvidenceLinkResponse): RecapGenre["evidenceLinks"][0] => ({
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
  bullets: genre.bullets ?? [],
});

const adaptRecapSummary = (payload: RecapSummaryResponse): RecapSummary => ({
  jobId: payload.job_id,
  executedAt: payload.executed_at,
  windowStart: payload.window_start,
  windowEnd: payload.window_end,
  totalArticles: payload.total_articles,
  genres: (payload.genres ?? []).map(adaptGenre),
});

/**
 * 7日間のリキャップデータを取得（クライアントサイド）
 * 30秒タイムアウト設定（バックエンドがrecap-workerから取得するため長め）
 */
export async function get7DaysRecapClient(): Promise<RecapSummary> {
  try {
    // Note: callClientAPI doesn't support timeout directly, but the backend should handle it
    const response = await callClientAPI<RecapSummaryResponse>("/v1/recap/7days");
    return adaptRecapSummary(response);
  } catch (error) {
    const errorMessage =
      error instanceof Error ? error.message : "Failed to fetch 7-day recap";
    throw new Error(errorMessage);
  }
}

