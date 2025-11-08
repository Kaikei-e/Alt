import type { ApiClient } from "../core/ApiClient";
import { ApiError } from "../core/ApiError";
import type { RecapSummary } from "@/schema/recap";

export class RecapApi {
  constructor(private apiClient: ApiClient) {}

  async get7DaysRecap(): Promise<RecapSummary> {
    try {
      const response = await this.apiClient.get<RecapSummary>(
        "/v1/recap/7days",
        30 // 30秒タイムアウト（バックエンドがrecap-workerから取得するため長め）
      );
      return response;
    } catch (error) {
      throw new ApiError("Failed to fetch 7-day recap", 500, error as string);
    }
  }
}

