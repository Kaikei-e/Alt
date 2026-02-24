"use client";

import { useState } from "react";
import LazyRightPanel from "@/components/desktop/analytics/LazyRightPanel";
import { DesktopHeader } from "@/components/desktop/layout/DesktopHeader";
import { DesktopLayout } from "@/components/desktop/layout/DesktopLayout";
import LazyDesktopTimeline from "@/components/desktop/timeline/LazyDesktopTimeline";
import type { FeedStatsSummary } from "@/schema/feedStats";

export default function FeedsView({
  data: _data,
}: {
  data: FeedStatsSummary | null;
}) {
  const [searchQuery, setSearchQuery] = useState("");

  const sidebarNavItems = [
    {
      id: 1,
      label: "Dashboard",
      iconName: "Home",
      href: "/desktop/home",
      active: false,
    },
    {
      id: 2,
      label: "Feeds",
      iconName: "Rss",
      href: "/desktop/feeds",
      active: true,
    },
    {
      id: 6,
      label: "Manage Feeds Links",
      iconName: "Link",
      href: "/feeds/manage",
    },
    {
      id: 3,
      label: "Articles",
      iconName: "FileText",
      href: "/desktop/articles",
    },
    {
      id: 4,
      label: "Search",
      iconName: "Search",
      href: "/desktop/articles/search",
    },
    {
      id: 5,
      label: "Settings",
      iconName: "Settings",
      href: "/desktop/settings",
    },
  ];

  return (
    <DesktopLayout
      showRightPanel={true}
      rightPanel={<LazyRightPanel />}
      sidebarProps={{
        navItems: sidebarNavItems,
        logoText: "Alt Dashboard",
        logoSubtext: "RSS Management Hub",
      }}
    >
      <DesktopHeader
        searchQuery={searchQuery}
        onSearchChange={setSearchQuery}
        currentTheme="vaporwave"
        onThemeToggle={() => {}}
      />
      <LazyDesktopTimeline />
    </DesktopLayout>
  );
}
