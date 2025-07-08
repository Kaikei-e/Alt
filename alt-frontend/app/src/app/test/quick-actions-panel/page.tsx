"use client";

import { Box, Text } from "@chakra-ui/react";
import { Suspense } from "react";
import { useSearchParams } from "next/navigation";
import { Plus, Search, Bookmark, Filter } from "lucide-react";
import QuickActionsPanel from "@/components/desktop/home/QuickActionsPanel";

function QuickActionsPanelTestContent() {
  const searchParams = useSearchParams();
  if (!searchParams) {
    return <Text>No search params</Text>;
  }
  const noStats = searchParams.get("noStats") === "true";

  const mockActions = [
    {
      id: 1,
      label: "Add Feed",
      icon: Plus,
      href: "/desktop/feeds/register",
    },
    {
      id: 2,
      label: "Search",
      icon: Search,
      href: "/desktop/search",
    },
    {
      id: 3,
      label: "Bookmarks",
      icon: Bookmark,
      href: "/desktop/bookmarks",
    },
    {
      id: 4,
      label: "Filter",
      icon: Filter,
      href: "/desktop/feeds?filter=unread",
    },
  ];

  const mockStats = noStats
    ? undefined
    : {
        weeklyReads: 156,
        aiProcessed: 78,
        bookmarks: 42,
      };

  return (
    <Box p={8} minH="100vh" bg="var(--app-bg)">
      <QuickActionsPanel actions={mockActions} additionalStats={mockStats} />
    </Box>
  );
}

export default function QuickActionsPanelTestPage() {
  return (
    <Suspense fallback={<div>Loading...</div>}>
      <QuickActionsPanelTestContent />
    </Suspense>
  );
}
