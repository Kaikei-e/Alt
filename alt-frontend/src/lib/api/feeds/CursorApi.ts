import type { CursorResponse } from "@/schema/common";
import type { ApiClient } from "../core/ApiClient";

export type CursorFetchFunction<T> = (
  cursor?: string,
  limit?: number,
) => Promise<CursorResponse<T>>;

export class CursorApi<BackendType, FrontendType> {
  constructor(
    private apiClient: ApiClient,
    private endpoint: string,
    private transformer: (item: BackendType) => FrontendType,
    private defaultCacheTtl: number = 10,
  ) {}

  async fetchWithCursor(
    cursor?: string,
    limit: number = 20,
  ): Promise<CursorResponse<FrontendType>> {
    // Validate limit constraints
    if (limit < 1 || limit > 100) {
      throw new Error("Limit must be between 1 and 100");
    }

    const params = new URLSearchParams();
    params.set("limit", limit.toString());
    if (cursor) {
      params.set("cursor", cursor);
    }

    // Use different cache TTL based on context
    const cacheTtl = cursor ? this.defaultCacheTtl + 5 : this.defaultCacheTtl;
    const response = await this.apiClient.get<CursorResponse<BackendType>>(
      `${this.endpoint}?${params.toString()}`,
      cacheTtl,
    );

    // Guard against null or malformed responses
    if (!response || !Array.isArray(response.data)) {
      return {
        data: [],
        next_cursor: null,
        has_more: false,
      };
    }

    // Transform backend items to frontend format
    const transformedData = response.data.map(this.transformer);

    const hasMore = response.has_more ?? response.next_cursor !== null;

    return {
      data: transformedData,
      next_cursor: response.next_cursor,
      has_more: hasMore,
    };
  }

  // Create a function compatible with the original API
  createFunction(): CursorFetchFunction<FrontendType> {
    return this.fetchWithCursor.bind(this);
  }
}
