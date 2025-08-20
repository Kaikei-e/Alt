// app/desktop/feeds/page.tsx
export const dynamic = 'force-dynamic'
export const fetchCache = 'force-no-store'  // 念のため
export const revalidate = 0

import React, { Suspense } from "react";
import { Box, Text } from "@chakra-ui/react";
import { DesktopLayout } from "@/components/desktop/layout/DesktopLayout";
import LazyDesktopTimeline from "@/components/desktop/timeline/LazyDesktopTimeline";
import LazyRightPanel from "@/components/desktop/analytics/LazyRightPanel";
import { Home, Rss, FileText, Search, Settings } from "lucide-react";
import { cookies } from 'next/headers';

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

function DesktopFeedsContent({ data }: { data: any }) {
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
  // TODO.md要件: Cookie明示転送でサーバfetch実行
  const cookie = (await cookies()).toString()
  const res = await fetch(process.env.API_URL + '/api/backend/v1/feeds/stats', { 
    headers: { cookie }, 
    cache: 'no-store' 
  })
  if (!res.ok) throw new Error('Failed to load feed stats')
  const data = await res.json()
  return (
    <Suspense fallback={<LoadingFallback />}>
      <DesktopFeedsContent data={data} />
    </Suspense>
  );
}
