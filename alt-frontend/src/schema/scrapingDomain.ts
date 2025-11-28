export interface ScrapingDomain {
  id: string;
  domain: string;
  scheme: string;
  allow_fetch_body: boolean;
  allow_ml_training: boolean;
  allow_cache_days: number;
  force_respect_robots: boolean;
  robots_txt_url?: string;
  robots_txt_content?: string;
  robots_txt_fetched_at?: string;
  robots_txt_last_status?: number;
  robots_crawl_delay_sec?: number;
  robots_disallow_paths: string[];
  created_at: string;
  updated_at: string;
}

export interface UpdateScrapingDomainRequest {
  allow_fetch_body?: boolean;
  allow_ml_training?: boolean;
  allow_cache_days?: number;
  force_respect_robots?: boolean;
}
