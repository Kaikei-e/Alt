import React from 'react';
import Link from 'next/link';
import { ChevronLeft, ChevronRight } from 'lucide-react';

interface NavItem {
  id: number;
  label: string;
  icon: React.ComponentType<{ size?: number }>;
  href: string;
  active?: boolean;
}

interface FeedSource {
  id: string;
  name: string;
  icon: string;
  unreadCount: number;
  category: string;
}

interface FilterState {
  readStatus: 'all' | 'read' | 'unread';
  sources: string[];
  priority: 'all' | 'high' | 'medium' | 'low';
  tags: string[];
  timeRange: 'all' | 'today' | 'week' | 'month';
}

interface DesktopSidebarProps {
  // Navigation props
  navItems?: NavItem[];
  logoText?: string;
  logoSubtext?: string;

  // Feeds filtering props
  activeFilters?: FilterState;
  onFilterChange?: (filters: FilterState) => void;
  feedSources?: FeedSource[];
  isCollapsed?: boolean;
  onToggleCollapse?: () => void;

  // Mode prop to determine which interface to use
  mode?: 'navigation' | 'feeds-filter';
}

export const DesktopSidebar: React.FC<DesktopSidebarProps> = ({
  navItems = [],
  logoText = 'Alt RSS',
  logoSubtext = 'Feed Reader',
  activeFilters,
  onFilterChange,
  feedSources = [],
  isCollapsed = false,
  onToggleCollapse,
  mode = 'navigation'
}) => {
  if (mode === 'feeds-filter') {
    return (
      <div className="glass h-full flex flex-col p-6">
        {/* Header with collapse toggle */}
        <div className="flex justify-between items-center mb-6">
          <h2 className="text-lg font-bold text-gray-900">Filters</h2>
          {onToggleCollapse && (
            <button
              onClick={onToggleCollapse}
              className="p-2 hover:bg-gray-100 rounded-lg transition-colors"
              aria-label="Collapse sidebar"
            >
              {isCollapsed ? (
                <ChevronRight size={16} />
              ) : (
                <ChevronLeft size={16} />
              )}
            </button>
          )}
        </div>

        {!isCollapsed && (
          <div className="flex-1 space-y-6">
            {/* Read Status Filter */}
            <div>
              <h3 className="text-sm font-medium text-gray-700 mb-3">Read Status</h3>
              <div className="space-y-2">
                {['all', 'unread', 'read'].map((status) => (
                  <label key={status} className="flex items-center">
                    <input
                      type="radio"
                      name="readStatus"
                      value={status}
                      checked={activeFilters?.readStatus === status}
                      onChange={() => onFilterChange?.({
                        ...activeFilters!,
                        readStatus: status as 'all' | 'read' | 'unread'
                      })}
                      className="mr-2"
                    />
                    <span className="text-sm capitalize">{status}</span>
                  </label>
                ))}
              </div>
            </div>

            {/* Feed Sources */}
            <div>
              <h3 className="text-sm font-medium text-gray-700 mb-3">Sources</h3>
              <div className="space-y-2 max-h-40 overflow-y-auto">
                {feedSources.map((source) => (
                  <label key={source.id} className="flex items-center justify-between">
                    <div className="flex items-center">
                      <input
                        type="checkbox"
                        checked={activeFilters?.sources.includes(source.id)}
                        onChange={(e) => {
                          const newSources = e.target.checked
                            ? [...(activeFilters?.sources || []), source.id]
                            : (activeFilters?.sources || []).filter(id => id !== source.id);
                          onFilterChange?.({
                            ...activeFilters!,
                            sources: newSources
                          });
                        }}
                        className="mr-2"
                      />
                      <span className="text-sm">{source.name}</span>
                    </div>
                    <span className="text-xs bg-gray-100 px-2 py-1 rounded">
                      {source.unreadCount}
                    </span>
                  </label>
                ))}
              </div>
            </div>

            {/* Time Range Filter */}
            <div>
              <h3 className="text-sm font-medium text-gray-700 mb-3">Time Range</h3>
              <div className="space-y-2">
                {['all', 'today', 'week', 'month'].map((range) => (
                  <label key={range} className="flex items-center">
                    <input
                      type="radio"
                      name="timeRange"
                      value={range}
                      checked={activeFilters?.timeRange === range}
                      onChange={() => onFilterChange?.({
                        ...activeFilters!,
                        timeRange: range as 'all' | 'today' | 'week' | 'month'
                      })}
                      className="mr-2"
                    />
                    <span className="text-sm capitalize">{range}</span>
                  </label>
                ))}
              </div>
            </div>

            {/* Clear Filters Button */}
            <button
              onClick={() => onFilterChange?.({
                sources: [],
                timeRange: 'all',
                readStatus: 'all',
                tags: [],
                priority: 'all'
              })}
              className="w-full px-3 py-2 text-sm border border-gray-300 rounded-lg hover:bg-gray-50 transition-colors"
            >
              Clear Filters
            </button>
          </div>
        )}
      </div>
    );
  }

  // Default navigation mode
  return (
    <div className="glass h-full flex flex-col p-6">
      {/* Logo Section */}
      <div className="mb-8">
        <h1 className="text-2xl font-bold text-gray-900 mb-1">
          {logoText}
        </h1>
        <p className="text-sm text-gray-600">
          {logoSubtext}
        </p>
      </div>

      {/* Navigation */}
      <nav className="flex-1" aria-label="Main navigation">
        <ul className="space-y-2">
          {navItems.map((item) => (
            <li key={item.id}>
              <Link
                href={item.href}
                className={`flex items-center gap-3 px-4 py-3 rounded-lg transition-colors ${
                  item.active
                    ? 'bg-purple-100 text-purple-700 active'
                    : 'text-gray-700 hover:bg-gray-100'
                }`}
              >
                <item.icon size={20} />
                <span className="font-medium">{item.label}</span>
              </Link>
            </li>
          ))}
        </ul>
      </nav>
    </div>
  );
};