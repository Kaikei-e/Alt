import React from 'react';

export interface DesktopFeedsLayoutProps {
  children: React.ReactNode;
  sidebar: React.ReactNode;
  header: React.ReactNode;
}

export interface DesktopHeaderProps {
  totalUnread: number;
  searchQuery?: string;
  onSearchChange: (query: string) => void;
  currentTheme: 'vaporwave' | 'liquid-beige';
  onThemeToggle: () => void;
}

export interface DesktopSidebarProps {
  navItems?: Array<{
    id: number;
    label: string;
    icon: React.ComponentType<{ size?: number }>;
    href: string;
    active?: boolean;
  }>;
  logoText?: string;
  logoSubtext?: string;

  activeFilters?: FilterState;
  onFilterChange?: (filters: FilterState) => void;
  onClearAll?: () => void;
  feedSources?: FeedSource[];
  isCollapsed?: boolean;
  onToggleCollapse?: () => void;

  mode?: 'navigation' | 'feeds-filter';
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