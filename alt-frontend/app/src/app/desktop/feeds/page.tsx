"use client";

import React, { Suspense } from "react";
import { Box, Text } from "@chakra-ui/react";
import { DesktopLayout } from "@/components/desktop/layout/DesktopLayout";
import DesktopTimeline from "@/components/desktop/timeline/DesktopTimeline";
import { RightPanel } from "@/components/desktop/analytics/RightPanel";
import { Home, Rss, FileText, Search, Settings } from "lucide-react";

// Loading fallback
const LoadingFallback = () => (
  <Box
    h="100vh"
    display="flex"
    alignItems="center"
    justifyContent="center"
    bg="var(--app-bg)"
  >
    <Box
      className="glass"
      p={8}
      borderRadius="var(--radius-xl)"
      textAlign="center"
    >
      <div
        style={{
          width: "32px",
          height: "32px",
          border: "3px solid var(--surface-border)",
          borderTop: "3px solid var(--accent-primary)",
          borderRadius: "50%",
          animation: "spin 1s linear infinite",
          margin: "0 auto 16px",
        }}
      />
      <Text color="var(--text-primary)" fontSize="lg">
        Loading Alt Feeds...
      </Text>
    </Box>
  </Box>
);

function DesktopFeedsContent() {
  const sidebarNavItems = [
    {
      id: 1,
      label: "Dashboard",
      icon: Home,
      href: "/desktop/home",
      active: false,
    },
    { id: 2, label: "Feeds", icon: Rss, href: "/desktop/feeds", active: true },
    { id: 3, label: "Articles", icon: FileText, href: "/desktop/articles" },
    { id: 4, label: "Search", icon: Search, href: "/desktop/articles/search" },
    { id: 5, label: "Settings", icon: Settings, href: "/desktop/settings" },
  ];

  return (
    <DesktopLayout
      showRightPanel={true}
      rightPanel={<RightPanel />}
      sidebarProps={{
        navItems: sidebarNavItems,
        logoText: "Alt Dashboard",
        logoSubtext: "RSS Management Hub",
      }}
    >
      <DesktopTimeline />
    </DesktopLayout>
  );
}

export default function DesktopFeedsPage() {
  return (
    <Suspense fallback={<LoadingFallback />}>
      <DesktopFeedsContent />
    </Suspense>
  );
}
