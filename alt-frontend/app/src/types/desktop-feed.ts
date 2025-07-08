import { Feed } from "@/schema/feed";

export interface DesktopFeed extends Feed {
  metadata: FeedMetadata;
  isRead: boolean;
  isFavorited: boolean;
  isBookmarked: boolean;
  readingProgress?: number; // 0-100
}

export interface FeedMetadata {
  source: FeedSource;
  readingTime: number;
  engagement: EngagementStats;
  tags: string[];
  relatedCount: number;
  publishedAt: string;
  author?: string;
  summary?: string;
  priority: "high" | "medium" | "low";
  category: string;
  difficulty: "beginner" | "intermediate" | "advanced";
}

export interface EngagementStats {
  // RSS-specific engagement stats only
  // views: number;     // Removed: SNS element
  // comments: number;  // Removed: SNS element
  likes: number;
  bookmarks: number;
}

export interface FeedSource {
  id: string;
  name: string;
  icon: string;
  reliability: number;
  category: string;
  unreadCount: number;
  avgReadingTime: number;
}

export interface DesktopFeedCardProps {
  feed: DesktopFeed;
  variant?: "default" | "compact" | "detailed";
  onMarkAsRead: (feedId: string) => void;
  onToggleFavorite: (feedId: string) => void;
  onToggleBookmark: (feedId: string) => void;
  onReadLater: (feedId: string) => void;
  onViewArticle: (feedId: string) => void;
}

export interface FilterState {
  readStatus: "all" | "read" | "unread";
  sources: string[];
  priority: "all" | "high" | "medium" | "low";
  tags: string[];
  timeRange: "all" | "today" | "week" | "month";
}

export interface DesktopFeedsResponse {
  feeds: Feed[];
  nextCursor: string | null;
  hasMore: boolean;
  totalCount: number;
}
