"use client";

import { BarChart3, Home, Rss, Settings } from "lucide-react";
import { useState } from "react";
import { DesktopSidebar } from "@/components/desktop/layout/DesktopSidebar";
import { ThemeProvider } from "@/providers/ThemeProvider";

interface FilterState {
  readStatus: "all" | "read" | "unread";
  sources: string[];
  priority: "all" | "high" | "medium" | "low";
  tags: string[];
  timeRange: "all" | "today" | "week" | "month";
}

export default function DesktopSidebarTest() {
  const [mode, setMode] = useState<"navigation" | "feeds-filter">("navigation");
  const [activeFilters, setActiveFilters] = useState<FilterState>({
    sources: [],
    timeRange: "all",
    readStatus: "all",
    tags: [],
    priority: "all",
  });
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);

  const mockNavItems = [
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
  ];

  const mockFeedSources = [
    {
      id: "techcrunch",
      name: "TechCrunch",
      icon: "📰",
      unreadCount: 12,
      category: "tech",
    },
    {
      id: "hackernews",
      name: "Hacker News",
      icon: "🔥",
      unreadCount: 8,
      category: "tech",
    },
    {
      id: "medium",
      name: "Medium",
      icon: "📝",
      unreadCount: 15,
      category: "general",
    },
    {
      id: "devto",
      name: "Dev.to",
      icon: "💻",
      unreadCount: 6,
      category: "development",
    },
    {
      id: "github",
      name: "GitHub",
      icon: "🐙",
      unreadCount: 4,
      category: "development",
    },
  ];

  return (
    <ThemeProvider>
      <div className="h-screen w-64 bg-gray-50">
        {/* Mode Toggle for Testing */}
        <div className="p-4 bg-white border-b">
          <div className="flex gap-2">
            <button
              onClick={() => setMode("navigation")}
              className={`px-3 py-1 rounded text-sm ${
                mode === "navigation" ? "bg-blue-500 text-white" : "bg-gray-200"
              }`}
            >
              Navigation
            </button>
            <button
              onClick={() => setMode("feeds-filter")}
              className={`px-3 py-1 rounded text-sm ${
                mode === "feeds-filter"
                  ? "bg-blue-500 text-white"
                  : "bg-gray-200"
              }`}
            >
              Feeds Filter
            </button>
          </div>
        </div>

        {/* Sidebar */}
        <div>
          {mode === "navigation" ? (
            <DesktopSidebar navItems={mockNavItems} mode="navigation" />
          ) : (
            <DesktopSidebar
              mode="feeds-filter"
              activeFilters={activeFilters}
              onFilterChange={setActiveFilters}
              feedSources={mockFeedSources}
              isCollapsed={sidebarCollapsed}
              onToggleCollapse={() => setSidebarCollapsed(!sidebarCollapsed)}
            />
          )}
        </div>
      </div>
    </ThemeProvider>
  );
}
