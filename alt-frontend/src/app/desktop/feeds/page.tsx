// app/desktop/feeds/page.tsx
export const dynamic = 'force-dynamic'
export const fetchCache = 'force-no-store'  // 念のため
export const revalidate = 0

import React, { Suspense } from "react";
import { Box, Text } from "@chakra-ui/react";
import { DesktopLayout } from "@/components/desktop/layout/DesktopLayout";
import LazyDesktopTimeline from "@/components/desktop/timeline/LazyDesktopTimeline";
import LazyRightPanel from "@/components/desktop/analytics/LazyRightPanel";
// Remove Lucide imports from Server Component
import { serverFetch } from '@/lib/server-fetch';
import { FeedStatsSummary } from '@/schema/feedStats';

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

function DesktopFeedsContent({ data }: { data: FeedStatsSummary | null }) {
  const sidebarNavItems = [
    {
      id: 1,
      label: "Dashboard",
      iconName: "Home",
      href: "/desktop/home",
      active: false,
    },
    { id: 2, label: "Feeds", iconName: "Rss", href: "/desktop/feeds", active: true },
    { id: 3, label: "Articles", iconName: "FileText", href: "/desktop/articles" },
    { id: 4, label: "Search", iconName: "Search", href: "/desktop/articles/search" },
    { id: 5, label: "Settings", iconName: "Settings", href: "/desktop/settings" },
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
      <LazyDesktopTimeline />
    </DesktopLayout>
  );
}

export default async function Page() {
  let data = null
  try {
    data = await serverFetch<FeedStatsSummary>('/v1/feeds/stats')
  } catch (e) {
    console.error('feed stats SSR failed:', e)
  }
  return (
    <Suspense fallback={<LoadingFallback />}>
      <DesktopFeedsContent data={data} />
    </Suspense>
  )
}
