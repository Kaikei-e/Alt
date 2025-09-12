export interface ApiConfig {
  baseUrl: string;
  defaultCacheTtl: number;
  requestTimeout: number;
  maxRetries: number;
}

export const defaultApiConfig: ApiConfig = {
  // TODO.md修正: SSR内向き vs Client外向き分離
  baseUrl:
    typeof window === "undefined"
      ? process.env.API_URL || "http://localhost:9000" // SSR: 内向き
      : "/api/backend", // Client: 外向き（Nginx書き換え）
  defaultCacheTtl: 5, // minutes
  requestTimeout: 30000, // 30 seconds
  maxRetries: 3,
};

export interface CacheConfig {
  defaultTtl: number;
  maxSize: number;
  cleanupInterval: number;
}

export const defaultCacheConfig: CacheConfig = {
  defaultTtl: 5 * 60 * 1000, // 5 minutes in milliseconds
  maxSize: 100, // maximum cache entries
  cleanupInterval: 10 * 60 * 1000, // 10 minutes in milliseconds
};

export interface SseConfig {
  maxReconnectAttempts: number;
  reconnectDelay: number;
  baseUrl: string;
}

import { PUBLIC_API_BASE_URL } from "@/lib/env.public";

export const defaultSseConfig: SseConfig = {
  maxReconnectAttempts: 5,
  reconnectDelay: 2000, // 2 seconds
  baseUrl: PUBLIC_API_BASE_URL,
};
