import type React from "react";

// 統計カード用の型
export interface StatsCardData {
  id: string;
  icon: React.ComponentType<{ size?: number }>;
  label: string;
  value: number;
  trend?: string;
  trendLabel?: string;
  color: "primary" | "secondary" | "tertiary";
}

// アクティビティ用の型
export interface ActivityData {
  id: number;
  type: "new_feed" | "ai_summary" | "bookmark" | "read";
  title: string;
  time: string;
}

// クイックアクション用の型
export interface QuickActionData {
  id: number;
  label: string;
  icon: React.ComponentType<{ size?: number }>;
  href: string;
}

// API Activity Response 用の型
export interface ActivityResponse {
  id: number;
  type: "new_feed" | "ai_summary" | "bookmark" | "read";
  title: string;
  timestamp: string;
}

// Weekly Stats 用の型
export interface WeeklyStats {
  weeklyReads: number;
  aiProcessed: number;
  bookmarks: number;
}
