import { callClientAPI } from "./core";
import type { FeedLink } from "$lib/schema/feedLink";

/**
 * RSSフィードリンク一覧を取得（クライアントサイド）
 */
export async function listFeedLinksClient(): Promise<FeedLink[]> {
  return callClientAPI<FeedLink[]>("/v1/rss-feed-link/list");
}

/**
 * RSSフィードリンクを登録（クライアントサイド）
 */
export async function registerRssFeedClient(url: string): Promise<void> {
  await callClientAPI("/v1/rss-feed-link/register", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify({ url }),
  });
}

/**
 * RSSフィードリンクを削除（クライアントサイド）
 */
export async function deleteFeedLinkClient(id: string): Promise<void> {
  await callClientAPI(`/v1/rss-feed-link/${encodeURIComponent(id)}`, {
    method: "DELETE",
  });
}


