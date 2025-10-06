"use client";

import React from "react";
import { Box, Text, Spinner } from "@chakra-ui/react";
import { DesktopLayout } from "@/components/desktop/layout/DesktopLayout";
import { RightPanel } from "@/components/desktop/analytics/RightPanel";
import { Suspense } from "react";
import { DesktopArticleTimeline } from "@/components/desktop/timeline/DesktopArticleTimeline";

function DesktopArticlesContent() {
  return (
    <DesktopLayout>
      <DesktopArticleTimeline />
      <RightPanel />
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
      <Spinner
        size="lg"
        color="var(--accent-primary)"
        mb={4}
      />
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
