"use client";

import { Box, Text } from "@chakra-ui/react";
import { Suspense } from "react";
import { useSearchParams } from "next/navigation";
import ActivityFeed from "@/components/desktop/home/ActivityFeed";

function ActivityFeedTestContent() {
  const searchParams = useSearchParams();
  if (!searchParams) {
    return <Text>No search params</Text>;
  }
  const isLoading = searchParams.get('loading') === 'true';
  const isEmpty = searchParams.get('empty') === 'true';

  const mockActivities: Array<{ id: number; type: "new_feed" | "ai_summary" | "bookmark" | "read"; title: string; time: string; }> = isEmpty ? [] : [
    { id: 1, type: "new_feed", title: "TechCrunch added", time: "2 min ago" },
    { id: 2, type: "ai_summary", title: "5 articles summarized", time: "5 min ago" },
    { id: 3, type: "bookmark", title: "Article bookmarked", time: "12 min ago" },
    { id: 4, type: "read", title: "15 articles read", time: "1 hour ago" },
  ];

  return (
    <Box p={8} minH="100vh" bg="var(--app-bg)">
      <ActivityFeed
        activities={mockActivities}
        isLoading={isLoading}
      />
    </Box>
  );
}

export default function ActivityFeedTestPage() {
  return (
    <Suspense fallback={<div>Loading...</div>}>
      <ActivityFeedTestContent />
    </Suspense>
  );
}