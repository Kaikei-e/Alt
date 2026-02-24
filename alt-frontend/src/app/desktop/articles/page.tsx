"use client";

import { Box, Spinner, Text } from "@chakra-ui/react";
import { Suspense } from "react";
import { DesktopLayout } from "@/components/desktop/layout/DesktopLayout";
import { DesktopArticleTimeline } from "@/components/desktop/timeline/DesktopArticleTimeline";

function DesktopArticlesContent() {
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
      active: false,
    },
    {
      id: 3,
      label: "Articles",
      iconName: "FileText",
      href: "/desktop/articles",
      active: true,
    },
    {
      id: 4,
      label: "Search",
      iconName: "Search",
      href: "/desktop/articles/search",
      active: false,
    },
    {
      id: 5,
      label: "Settings",
      iconName: "Settings",
      href: "/desktop/settings",
      active: false,
    },
  ];

  return (
    <DesktopLayout
      sidebarProps={{
        navItems: sidebarNavItems,
        logoText: "Alt Dashboard",
        logoSubtext: "RSS Management Hub",
      }}
    >
      <DesktopArticleTimeline />
    </DesktopLayout>
  );
}

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
      <Spinner size="lg" color="var(--accent-primary)" mb={4} />
      <Text color="var(--text-primary)" fontSize="lg">
        Loading Alt Articles...
      </Text>
    </Box>
  </Box>
);

export default function DesktopArticlesPage() {
  return (
    <Suspense fallback={<LoadingFallback />}>
      <DesktopArticlesContent />
    </Suspense>
  );
}
