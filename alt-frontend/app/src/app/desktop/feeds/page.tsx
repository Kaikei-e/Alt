'use client';

import React, { useState } from 'react';
import { DesktopFeedsLayout } from '@/components/desktop/layout/DesktopFeedsLayout';
import { DesktopHeader } from '@/components/desktop/layout/DesktopHeader';
import { DesktopSidebar } from '@/components/desktop/layout/DesktopSidebar';
import { DesktopTimeline } from '@/components/desktop/timeline/DesktopTimeline';
import { FilterState } from '@/types/desktop-feeds';

// モックデータ
const mockFeedSources = [
  { id: 'techcrunch', name: 'TechCrunch', icon: '📰', unreadCount: 12, category: 'tech' },
  { id: 'hackernews', name: 'Hacker News', icon: '🔥', unreadCount: 8, category: 'tech' },
  { id: 'medium', name: 'Medium', icon: '📝', unreadCount: 15, category: 'general' },
  { id: 'devto', name: 'Dev.to', icon: '💻', unreadCount: 6, category: 'development' },
  { id: 'github', name: 'GitHub', icon: '🐙', unreadCount: 4, category: 'development' }
];

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
  

  return (
    <DesktopFeedsLayout
      header={
        <DesktopHeader
          totalUnread={42}
          onSearchChange={setSearchQuery}
          currentTheme={'vaporwave'}
          onThemeToggle={() => {}}
        />
      }
      sidebar={
        <DesktopSidebar
          activeFilters={activeFilters}
          onFilterChange={setActiveFilters}
          feedSources={mockFeedSources}
          isCollapsed={sidebarCollapsed}
          onToggleCollapse={() => setSidebarCollapsed(!sidebarCollapsed)}
        />
      }
    >
      <DesktopTimeline
        searchQuery={searchQuery}
        filters={activeFilters}
      />
    </DesktopFeedsLayout>
  );
}