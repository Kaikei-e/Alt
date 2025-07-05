import React from 'react';

export interface DesktopFeedsLayoutProps {
  children: React.ReactNode;
  sidebar: React.ReactNode;
  header: React.ReactNode;
}

export interface DesktopHeaderProps {
  totalUnread: number;
  onSearchChange: (query: string) => void;
  currentTheme: 'vaporwave' | 'liquid-beige';
  onThemeToggle: () => void;
}

export interface DesktopSidebarProps {
  activeFilters: FilterState;
  onFilterChange: (filters: FilterState) => void;
  feedSources: FeedSource[];
  isCollapsed: boolean;
  onToggleCollapse: () => void;
}

export interface FilterState {
  sources: string[];
  timeRange: 'today' | 'week' | 'month' | 'all';
  readStatus: 'all' | 'unread' | 'read';
  tags: string[];
  priority: 'all' | 'high' | 'medium' | 'low';
}

export interface FeedSource {
  id: string;
  name: string;
  icon: string;
  unreadCount: number;
  category: string;
}