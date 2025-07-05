import React from 'react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { DesktopSidebar } from './DesktopSidebar';
import { Home, Rss } from 'lucide-react';

// Mock Next.js Link component
vi.mock('next/link', () => ({
  default: ({ children, href, className }: { children: React.ReactNode; href: string; className?: string }) => (
    <a href={href} className={className}>
      {children}
    </a>
  ),
}));

describe('DesktopSidebar', () => {
  const mockNavItems = [
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
    }
  ];

  const mockFeedSources = [
    { id: 'techcrunch', name: 'TechCrunch', icon: '📰', unreadCount: 12, category: 'tech' },
    { id: 'hackernews', name: 'Hacker News', icon: '🔥', unreadCount: 8, category: 'tech' }
  ];

  const mockActiveFilters = {
    readStatus: 'all' as const,
    sources: [],
    priority: 'all' as const,
    tags: [],
    timeRange: 'all' as const
  };

  const mockOnFilterChange = vi.fn();
  const mockOnToggleCollapse = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('Navigation Mode', () => {
    it('should render navigation items correctly', () => {
      render(<DesktopSidebar navItems={mockNavItems} mode="navigation" />);

      expect(screen.getByText('Dashboard')).toBeInTheDocument();
      expect(screen.getByText('Feeds')).toBeInTheDocument();
      expect(screen.getByText('Alt RSS')).toBeInTheDocument();
      expect(screen.getByText('Feed Reader')).toBeInTheDocument();
    });

    it('should highlight active navigation item', () => {
      render(<DesktopSidebar navItems={mockNavItems} mode="navigation" />);

      const activeItem = screen.getByText('Dashboard').closest('a');
      expect(activeItem).toHaveClass('active');
    });

    it('should have proper accessibility attributes', () => {
      render(<DesktopSidebar navItems={mockNavItems} mode="navigation" />);

      const nav = screen.getByRole('navigation');
      expect(nav).toHaveAttribute('aria-label', 'Main navigation');
    });

    it('should apply glassmorphism styling', () => {
      render(<DesktopSidebar navItems={mockNavItems} mode="navigation" />);

      const sidebar = screen.getByText('Alt RSS').closest('.glass');
      expect(sidebar).toBeInTheDocument();
    });

    it('should use default props when not provided', () => {
      render(<DesktopSidebar mode="navigation" />);

      expect(screen.getByText('Alt RSS')).toBeInTheDocument();
      expect(screen.getByText('Feed Reader')).toBeInTheDocument();
    });

    it('should allow custom logo text and subtext', () => {
      render(
        <DesktopSidebar
          navItems={mockNavItems}
          mode="navigation"
          logoText="Custom Logo"
          logoSubtext="Custom Subtext"
        />
      );

      expect(screen.getByText('Custom Logo')).toBeInTheDocument();
      expect(screen.getByText('Custom Subtext')).toBeInTheDocument();
    });
  });

  describe('Feeds Filter Mode', () => {
    it('should render filters header and collapse toggle', () => {
      render(
        <DesktopSidebar
          mode="feeds-filter"
          activeFilters={mockActiveFilters}
          onFilterChange={mockOnFilterChange}
          feedSources={mockFeedSources}
          isCollapsed={false}
          onToggleCollapse={mockOnToggleCollapse}
        />
      );

      expect(screen.getByText('Filters')).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Collapse sidebar' })).toBeInTheDocument();
    });

    it('should display read status filter options', () => {
      render(
        <DesktopSidebar
          mode="feeds-filter"
          activeFilters={mockActiveFilters}
          onFilterChange={mockOnFilterChange}
          feedSources={mockFeedSources}
          isCollapsed={false}
          onToggleCollapse={mockOnToggleCollapse}
        />
      );

      expect(screen.getByText('Read Status')).toBeInTheDocument();
      expect(screen.getByLabelText('all')).toBeInTheDocument();
      expect(screen.getByLabelText('unread')).toBeInTheDocument();
      expect(screen.getByLabelText('read')).toBeInTheDocument();
    });

    it('should display feed sources with unread counts', () => {
      render(
        <DesktopSidebar
          mode="feeds-filter"
          activeFilters={mockActiveFilters}
          onFilterChange={mockOnFilterChange}
          feedSources={mockFeedSources}
          isCollapsed={false}
          onToggleCollapse={mockOnToggleCollapse}
        />
      );

      expect(screen.getByText('Sources')).toBeInTheDocument();
      expect(screen.getByText('TechCrunch')).toBeInTheDocument();
      expect(screen.getByText('12')).toBeInTheDocument();
      expect(screen.getByText('Hacker News')).toBeInTheDocument();
      expect(screen.getByText('8')).toBeInTheDocument();
    });

    it('should display time range filter options', () => {
      render(
        <DesktopSidebar
          mode="feeds-filter"
          activeFilters={mockActiveFilters}
          onFilterChange={mockOnFilterChange}
          feedSources={mockFeedSources}
          isCollapsed={false}
          onToggleCollapse={mockOnToggleCollapse}
        />
      );

      expect(screen.getByText('Time Range')).toBeInTheDocument();
      expect(screen.getByLabelText('all')).toBeInTheDocument();
      expect(screen.getByLabelText('today')).toBeInTheDocument();
      expect(screen.getByLabelText('week')).toBeInTheDocument();
      expect(screen.getByLabelText('month')).toBeInTheDocument();
    });

    it('should have clear filters button', () => {
      render(
        <DesktopSidebar
          mode="feeds-filter"
          activeFilters={mockActiveFilters}
          onFilterChange={mockOnFilterChange}
          feedSources={mockFeedSources}
          isCollapsed={false}
          onToggleCollapse={mockOnToggleCollapse}
        />
      );

      expect(screen.getByRole('button', { name: 'Clear Filters' })).toBeInTheDocument();
    });

    it('should handle read status filter changes', () => {
      render(
        <DesktopSidebar
          mode="feeds-filter"
          activeFilters={mockActiveFilters}
          onFilterChange={mockOnFilterChange}
          feedSources={mockFeedSources}
          isCollapsed={false}
          onToggleCollapse={mockOnToggleCollapse}
        />
      );

      const unreadRadio = screen.getByLabelText('unread');
      fireEvent.click(unreadRadio);

      expect(mockOnFilterChange).toHaveBeenCalledWith({
        ...mockActiveFilters,
        readStatus: 'unread'
      });
    });

    it('should handle source filter changes', () => {
      render(
        <DesktopSidebar
          mode="feeds-filter"
          activeFilters={mockActiveFilters}
          onFilterChange={mockOnFilterChange}
          feedSources={mockFeedSources}
          isCollapsed={false}
          onToggleCollapse={mockOnToggleCollapse}
        />
      );

      const techcrunchCheckbox = screen.getByLabelText('TechCrunch');
      fireEvent.click(techcrunchCheckbox);

      expect(mockOnFilterChange).toHaveBeenCalledWith({
        ...mockActiveFilters,
        sources: ['techcrunch']
      });
    });

    it('should handle time range filter changes', () => {
      render(
        <DesktopSidebar
          mode="feeds-filter"
          activeFilters={mockActiveFilters}
          onFilterChange={mockOnFilterChange}
          feedSources={mockFeedSources}
          isCollapsed={false}
          onToggleCollapse={mockOnToggleCollapse}
        />
      );

      const weekRadio = screen.getByLabelText('week');
      fireEvent.click(weekRadio);

      expect(mockOnFilterChange).toHaveBeenCalledWith({
        ...mockActiveFilters,
        timeRange: 'week'
      });
    });

    it('should handle sidebar collapse', () => {
      render(
        <DesktopSidebar
          mode="feeds-filter"
          activeFilters={mockActiveFilters}
          onFilterChange={mockOnFilterChange}
          feedSources={mockFeedSources}
          isCollapsed={false}
          onToggleCollapse={mockOnToggleCollapse}
        />
      );

      const collapseButton = screen.getByRole('button', { name: 'Collapse sidebar' });
      fireEvent.click(collapseButton);

      expect(mockOnToggleCollapse).toHaveBeenCalled();
    });

    it('should hide filter content when collapsed', () => {
      render(
        <DesktopSidebar
          mode="feeds-filter"
          activeFilters={mockActiveFilters}
          onFilterChange={mockOnFilterChange}
          feedSources={mockFeedSources}
          isCollapsed={true}
          onToggleCollapse={mockOnToggleCollapse}
        />
      );

      expect(screen.queryByText('Read Status')).not.toBeInTheDocument();
      expect(screen.queryByText('Sources')).not.toBeInTheDocument();
      expect(screen.queryByText('Time Range')).not.toBeInTheDocument();
    });

    it('should clear all filters when clear button is clicked', () => {
      const filtersWithValues = {
        ...mockActiveFilters,
        readStatus: 'unread' as const,
        sources: ['techcrunch'],
        timeRange: 'week' as const
      };

      render(
        <DesktopSidebar
          mode="feeds-filter"
          activeFilters={filtersWithValues}
          onFilterChange={mockOnFilterChange}
          feedSources={mockFeedSources}
          isCollapsed={false}
          onToggleCollapse={mockOnToggleCollapse}
        />
      );

      const clearButton = screen.getByRole('button', { name: 'Clear Filters' });
      fireEvent.click(clearButton);

      expect(mockOnFilterChange).toHaveBeenCalledWith({
        sources: [],
        timeRange: 'all',
        readStatus: 'all',
        tags: [],
        priority: 'all'
      });
    });

    it('should apply glassmorphism styling in filter mode', () => {
      render(
        <DesktopSidebar
          mode="feeds-filter"
          activeFilters={mockActiveFilters}
          onFilterChange={mockOnFilterChange}
          feedSources={mockFeedSources}
          isCollapsed={false}
          onToggleCollapse={mockOnToggleCollapse}
        />
      );

      const sidebar = screen.getByText('Filters').closest('.glass');
      expect(sidebar).toBeInTheDocument();
    });

    it('should handle multiple source selections', () => {
      const filtersWithSource = {
        ...mockActiveFilters,
        sources: ['techcrunch']
      };

      render(
        <DesktopSidebar
          mode="feeds-filter"
          activeFilters={filtersWithSource}
          onFilterChange={mockOnFilterChange}
          feedSources={mockFeedSources}
          isCollapsed={false}
          onToggleCollapse={mockOnToggleCollapse}
        />
      );

      const hackernewsCheckbox = screen.getByLabelText('Hacker News');
      fireEvent.click(hackernewsCheckbox);

      expect(mockOnFilterChange).toHaveBeenCalledWith({
        ...filtersWithSource,
        sources: ['techcrunch', 'hackernews']
      });
    });

    it('should remove source when unchecked', () => {
      const filtersWithSources = {
        ...mockActiveFilters,
        sources: ['techcrunch', 'hackernews']
      };

      render(
        <DesktopSidebar
          mode="feeds-filter"
          activeFilters={filtersWithSources}
          onFilterChange={mockOnFilterChange}
          feedSources={mockFeedSources}
          isCollapsed={false}
          onToggleCollapse={mockOnToggleCollapse}
        />
      );

      const techcrunchCheckbox = screen.getByLabelText('TechCrunch');
      fireEvent.click(techcrunchCheckbox);

      expect(mockOnFilterChange).toHaveBeenCalledWith({
        ...filtersWithSources,
        sources: ['hackernews']
      });
    });
  });

  describe('Edge Cases', () => {
    it('should handle empty feed sources gracefully', () => {
      render(
        <DesktopSidebar
          mode="feeds-filter"
          activeFilters={mockActiveFilters}
          onFilterChange={mockOnFilterChange}
          feedSources={[]}
          isCollapsed={false}
          onToggleCollapse={mockOnToggleCollapse}
        />
      );

      expect(screen.getByText('Sources')).toBeInTheDocument();
      expect(screen.queryByText('TechCrunch')).not.toBeInTheDocument();
    });

    it('should handle missing optional props gracefully', () => {
      render(<DesktopSidebar mode="feeds-filter" />);

      expect(screen.getByText('Filters')).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Collapse sidebar' })).toBeInTheDocument();
    });

    it('should not call onFilterChange when not provided', () => {
      render(
        <DesktopSidebar
          mode="feeds-filter"
          activeFilters={mockActiveFilters}
          feedSources={mockFeedSources}
          isCollapsed={false}
          onToggleCollapse={mockOnToggleCollapse}
        />
      );

      const unreadRadio = screen.getByLabelText('unread');
      fireEvent.click(unreadRadio);

      // Should not throw error
      expect(unreadRadio).toBeInTheDocument();
    });

    it('should not call onToggleCollapse when not provided', () => {
      render(
        <DesktopSidebar
          mode="feeds-filter"
          activeFilters={mockActiveFilters}
          onFilterChange={mockOnFilterChange}
          feedSources={mockFeedSources}
          isCollapsed={false}
        />
      );

      const collapseButton = screen.getByRole('button', { name: 'Collapse sidebar' });
      fireEvent.click(collapseButton);

      // Should not throw error
      expect(collapseButton).toBeInTheDocument();
    });
  });
});