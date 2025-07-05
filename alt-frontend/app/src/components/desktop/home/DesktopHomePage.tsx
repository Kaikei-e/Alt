import React from 'react';
import { DesktopLayout } from '../layout/DesktopLayout';
import { PageHeader } from './PageHeader';
import { StatsGrid } from './StatsGrid';
import { ActivityFeed } from './ActivityFeed';
import { QuickActionsPanel } from './QuickActionsPanel';
import { CallToActionBar } from './CallToActionBar';
import { Home, Rss, BarChart3, Settings, ArrowRight, Download, Users, Eye, Clock, Plus, Search, Bookmark, Filter } from 'lucide-react';

export const DesktopHomePage: React.FC = () => {
  // Mock sidebar navigation items
  const sidebarProps = {
    navItems: [
      {
        id: 1,
        label: 'Dashboard',
        icon: Home,
        href: '/desktop',
        active: true
      },
      {
        id: 2,
        label: 'Feeds',
        icon: Rss,
        href: '/desktop/feeds',
        active: false
      },
      {
        id: 3,
        label: 'Statistics',
        icon: BarChart3,
        href: '/desktop/stats',
        active: false
      },
      {
        id: 4,
        label: 'Settings',
        icon: Settings,
        href: '/desktop/settings',
        active: false
      }
    ]
  };

  // Mock stats data
  const mockStats = [
    {
      id: 'total-feeds',
      icon: Rss,
      label: 'Total Feeds',
      value: 24,
      trend: '+12%',
      trendLabel: 'vs last month',
      color: 'primary' as const
    },
    {
      id: 'unread-articles',
      icon: Eye,
      label: 'Unread Articles',
      value: 156,
      trend: '+5%',
      trendLabel: 'vs yesterday',
      color: 'secondary' as const
    },
    {
      id: 'reading-time',
      icon: Clock,
      label: 'Reading Time',
      value: 45,
      trend: '+2h',
      trendLabel: 'this week',
      color: 'tertiary' as const
    }
  ];

  // Mock activity data
  const mockActivities = [
    {
      id: 1,
      type: 'new_feed' as const,
      title: 'Added TechCrunch RSS feed',
      time: '2 hours ago'
    },
    {
      id: 2,
      type: 'ai_summary' as const,
      title: 'AI summary generated for 5 articles',
      time: '4 hours ago'
    },
    {
      id: 3,
      type: 'bookmark' as const,
      title: 'Bookmarked "Introduction to React 19"',
      time: '1 day ago'
    }
  ];

  // Mock quick actions
  const mockQuickActions = [
    {
      id: 1,
      label: 'Add Feed',
      icon: Plus,
      href: '/desktop/feeds/register'
    },
    {
      id: 2,
      label: 'Search',
      icon: Search,
      href: '/desktop/search'
    },
    {
      id: 3,
      label: 'Bookmarks',
      icon: Bookmark,
      href: '/desktop/bookmarks'
    },
    {
      id: 4,
      label: 'Filter',
      icon: Filter,
      href: '/desktop/feeds?filter=unread'
    }
  ];

  // Mock CTA actions
  const ctaActions = [
    {
      label: "Browse Feeds",
      href: "/desktop/feeds",
      icon: ArrowRight
    },
    {
      label: "Add New Feed",
      href: "/desktop/feeds/register",
      icon: Download
    }
  ];

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
        
        <StatsGrid stats={mockStats} />
        
        <div className="grid grid-cols-2 gap-8">
          <div>
            <ActivityFeed activities={mockActivities} />
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