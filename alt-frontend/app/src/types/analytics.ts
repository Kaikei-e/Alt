export interface ReadingAnalytics {
  today: DailyStats;
  week: WeeklyStats;
  month: MonthlyStats;
  streak: ReadingStreak;
}

export interface DailyStats {
  articlesRead: number;
  timeSpent: number; // minutes
  favoriteCount: number;
  completionRate: number; // percentage
  avgReadingTime: number;
  topCategories: CategoryStat[];
}

export interface WeeklyStats {
  totalArticles: number;
  totalTime: number;
  dailyBreakdown: DailyBreakdown[];
  trendDirection: "up" | "down" | "stable";
  weekOverWeek: number; // percentage change
}

export interface MonthlyStats {
  totalArticles: number;
  totalTime: number;
  monthlyGoal?: number;
  progress: number; // percentage of goal
}

export interface ReadingStreak {
  current: number;
  longest: number;
  lastReadDate: string;
}

export interface DailyBreakdown {
  day: string;
  articles: number;
  timeSpent: number;
  completion: number;
}

export interface CategoryStat {
  category: string;
  count: number;
  percentage: number;
  color: string;
}

export interface TrendingTopic {
  tag: string;
  count: number;
  trend: "up" | "down" | "stable";
  trendValue: number; // percentage change
  category: string;
  color: string;
}

export interface SourceAnalytic {
  id: string;
  name: string;
  icon: string;
  unreadCount: number;
  totalArticles: number;
  avgReadingTime: number;
  reliability: number;
  lastUpdate: string;
  engagement: number;
  category: string;
}

export interface QuickAction {
  id: string;
  title: string;
  icon: string;
  count?: number;
  action: () => void;
  color?: string;
}
