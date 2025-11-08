// RecapジャンルデータとEvidence（証拠記事）の型定義
export type RecapGenre = {
  genre: string;                    // AI, Business, Entertainment等
  summary: string;                  // 日本語要約（最終サマリー）
  topTerms: string[];              // 上位トピック（3〜5個）
  articleCount: number;            // 関連記事数
  clusterCount: number;            // クラスター数
  evidenceLinks: EvidenceLink[];   // 証拠記事（先読み5件程度）
};

export type EvidenceLink = {
  articleId: string;
  title: string;
  sourceUrl: string;
  publishedAt: string;             // ISO 8601
  lang: 'en' | 'ja' | string;
};

export type RecapSummary = {
  jobId: string;
  executedAt: string;              // ISO 8601
  windowStart: string;             // ISO 8601
  windowEnd: string;               // ISO 8601
  totalArticles: number;
  genres: RecapGenre[];            // 10件
};

