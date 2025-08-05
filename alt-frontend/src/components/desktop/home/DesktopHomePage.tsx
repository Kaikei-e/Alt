import React, { useMemo } from "react";
import { DesktopLayout } from "../layout/DesktopLayout";
import { PageHeader } from "./PageHeader";
import { StatsGrid } from "./StatsGrid";
import { ActivityFeed } from "./ActivityFeed";
import { QuickActionsPanel } from "./QuickActionsPanel";
import { CallToActionBar } from "./CallToActionBar";
import { useHomeStats } from "@/hooks/useHomeStats";
import { useRecentActivity } from "@/hooks/useRecentActivity";
import { transformFeedStats } from "@/utils/dataTransformers";
import {
  Home,
  Rss,
  BarChart3,
  Settings,
  ArrowRight,
  Download,
  Plus,
  Search,
  Bookmark,
  Filter,
} from "lucide-react";

export const DesktopHomePage: React.FC = () => {
  // Fetch real data from hooks
  const { feedStats, isLoadingStats, statsError, unreadCount, refreshStats } =
    useHomeStats();

  const {
    activities,
    isLoading: isLoadingActivity,
    error: activityError,
  } = useRecentActivity();

  // Transform feed stats to stats card format
  const statsData = useMemo(
    () => transformFeedStats(feedStats, unreadCount),
    [feedStats, unreadCount],
  );

  // Mock sidebar navigation items (keep as before since this doesn't come from API)
  const sidebarProps = {
    navItems: [
      {
        id: 1,
        label: "Dashboard",
        icon: Home,
        href: "/desktop",
        active: true,
      },
      {
        id: 2,
        label: "Feeds",
        icon: Rss,
        href: "/desktop/feeds",
        active: false,
      },
      {
        id: 3,
        label: "Statistics",
        icon: BarChart3,
        href: "/desktop/stats",
        active: false,
      },
      {
        id: 4,
        label: "Settings",
        icon: Settings,
        href: "/desktop/settings",
        active: false,
      },
    ],
  };

  // Mock quick actions
  const mockQuickActions = [
    {
      id: 1,
      label: "Add Feed",
      icon: Plus,
      href: "/desktop/feeds/register",
    },
    {
      id: 2,
      label: "Search",
      icon: Search,
      href: "/desktop/search",
    },
    {
      id: 3,
      label: "Bookmarks",
      icon: Bookmark,
      href: "/desktop/bookmarks",
    },
    {
      id: 4,
      label: "Filter",
      icon: Filter,
      href: "/desktop/feeds?filter=unread",
    },
  ];

  // Mock CTA actions
  const ctaActions = [
    {
      label: "Browse Feeds",
      href: "/desktop/feeds",
      icon: ArrowRight,
    },
    {
      label: "Add New Feed",
      href: "/desktop/feeds/register",
      icon: Download,
    },
  ];

  // Handle error states
  if (statsError || activityError) {
    return (
      <DesktopLayout sidebarProps={sidebarProps}>
        <div
          className="flex flex-col gap-8"
          data-testid="desktop-home-container"
        >
          <PageHeader
            title="Dashboard Overview"
            description="Monitor your RSS feeds and AI-powered content insights"
          />

          <div className="text-center py-8">
            <p className="text-red-500">{statsError || activityError}</p>
            <button
              onClick={refreshStats}
              className="mt-4 px-4 py-2 bg-blue-500 text-white rounded hover:bg-blue-600"
            >
              Retry
            </button>
          </div>
        </div>
      </DesktopLayout>
    );
  }

  return (
    <DesktopLayout sidebarProps={sidebarProps}>
      <div className="flex flex-col gap-8" data-testid="desktop-home-container">
        <PageHeader
          title="Dashboard Overview"
          description="Monitor your RSS feeds and AI-powered content insights"
        />

        <StatsGrid stats={statsData} isLoading={isLoadingStats} />

        <div className="grid grid-cols-2 gap-8">
          <div>
            <ActivityFeed
              activities={isLoadingActivity ? [] : activities}
              isLoading={isLoadingActivity}
            />
          </div>
          <div>
            <QuickActionsPanel actions={mockQuickActions} />
          </div>
        </div>

        <CallToActionBar
          title="Ready to explore?"
          description="Discover new content and manage your feeds"
          actions={ctaActions}
        />
      </div>
    </DesktopLayout>
  );
};
