"use client";

import { DesktopLayout } from "@/components/desktop/layout/DesktopLayout";
import { ThemeProvider } from "@/providers/ThemeProvider";
import { Home, Rss, BarChart3, Settings } from "lucide-react";

export default function DesktopLayoutTest() {
  const mockSidebarProps = {
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

  return (
    <ThemeProvider>
      <DesktopLayout sidebarProps={mockSidebarProps}>
        <div>
          <h1>Test Content</h1>
          <p>
            This is the main content area for testing the DesktopLayout
            component.
          </p>
        </div>
      </DesktopLayout>
    </ThemeProvider>
  );
}
