'use client';

import React, { useState } from 'react';
import { DesktopFeedsLayout } from '@/components/desktop/layout/DesktopFeedsLayout';
import { DesktopHeader } from '@/components/desktop/layout/DesktopHeader';
import { DesktopSidebar } from '@/components/desktop/layout/DesktopSidebar';
import { DesktopTimeline } from '@/components/desktop/timeline/DesktopTimeline';
import { FilterState } from '@/types/desktop-feed';
import { useURLFilters } from '@/hooks/useURLFilters';

// モックデータ
const mockFeedSources = [
  { id: 'techcrunch', name: 'TechCrunch', icon: '📰', unreadCount: 12, category: 'tech' },
  { id: 'hackernews', name: 'Hacker News', icon: '🔥', unreadCount: 8, category: 'tech' },
  { id: 'medium', name: 'Medium', icon: '📝', unreadCount: 15, category: 'general' },
  { id: 'devto', name: 'Dev.to', icon: '💻', unreadCount: 6, category: 'development' },
  { id: 'github', name: 'GitHub', icon: '🐙', unreadCount: 4, category: 'development' },
  { id: 'verge', name: 'The Verge', icon: '📱', unreadCount: 23, category: 'tech' },
  { id: 'wired', name: 'Wired', icon: '🔬', unreadCount: 7, category: 'science' },
  { id: 'bbc', name: 'BBC Tech', icon: '📺', unreadCount: 11, category: 'news' }
];

const mockStats = {
  totalUnread: 86,
  totalFeeds: 8,
  readToday: 12,
  weeklyAverage: 45
};

export default function DesktopFeedsPage() {
  const [searchQuery, setSearchQuery] = useState('');
  const [activeFilters, setActiveFilters] = useState<FilterState>({
    sources: [],
    timeRange: 'all',
    readStatus: 'all',
    tags: [],
    priority: 'all'
  });
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);

  // URL filter state management
  const { clearAllFilters: clearAllFromURL } = useURLFilters(
    activeFilters,
    setActiveFilters,
    searchQuery,
    setSearchQuery
  );


  return (
    <DesktopFeedsLayout
      header={
        <DesktopHeader
          totalUnread={mockStats.totalUnread}
          searchQuery={searchQuery}
          onSearchChange={setSearchQuery}
          currentTheme={'vaporwave'}
          onThemeToggle={() => {}}
        />
      }
      sidebar={
        <DesktopSidebar
          mode="feeds-filter"
          activeFilters={activeFilters}
          onFilterChange={setActiveFilters}
          onClearAll={clearAllFromURL}
          feedSources={mockFeedSources}
          isCollapsed={sidebarCollapsed}
          onToggleCollapse={() => setSidebarCollapsed(!sidebarCollapsed)}
        />
      }
    >
      <DesktopTimeline
        searchQuery={searchQuery}
        filters={activeFilters}
        onFilterChange={setActiveFilters}
      />
    </DesktopFeedsLayout>
  );
}