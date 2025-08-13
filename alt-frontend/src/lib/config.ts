export interface ApiConfig {
  baseUrl: string;
  defaultCacheTtl: number;
  requestTimeout: number;
  maxRetries: number;
}

export const defaultApiConfig: ApiConfig = {
  baseUrl: process.env.NEXT_PUBLIC_API_BASE_URL || "http://localhost:9000",
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

export const defaultSseConfig: SseConfig = {
  maxReconnectAttempts: 5,
  reconnectDelay: 2000, // 2 seconds
  baseUrl: process.env.NEXT_PUBLIC_API_BASE_URL || "http://localhost:9000",
};
