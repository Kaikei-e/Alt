import { useMemo } from "react";
import type { QuickAction } from "@/types/analytics";

export const useQuickActions = () => {
  const actions: QuickAction[] = useMemo(
    () => [
      {
        id: "view-unread",
        title: "View Unread",
        icon: "📖",
        count: 12,
        color: "var(--accent-primary)",
        action: () => {
          // フィルターを未読に設定
        },
      },
      {
        id: "view-bookmarks",
        title: "View Bookmarks",
        icon: "🔖",
        count: 5,
        color: "var(--accent-secondary)",
        action: () => {
          // ブックマーク画面に移動
        },
      },
      {
        id: "view-queue",
        title: "Reading Queue",
        icon: "📚",
        count: 8,
        color: "var(--accent-tertiary)",
        action: () => {
          // 読書キューを表示
        },
      },
      {
        id: "mark-all-read",
        title: "Mark All Read",
        icon: "✅",
        color: "var(--alt-success)",
        action: () => {
          // 全て既読にマーク
        },
      },
    ],
    []
  );

  const counters = {
    unread: 12,
    bookmarks: 5,
    queue: 8,
  };

  return { actions, counters };
};
