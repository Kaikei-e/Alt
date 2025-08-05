"use client";

import { PageHeader } from "@/components/desktop/home/PageHeader";
import { Box } from "@chakra-ui/react";

export default function PageHeaderTestPage() {
  return (
    <Box minHeight="100vh" bg="var(--app-bg)">
      <PageHeader
        title="Dashboard Overview"
        description="Monitor your RSS feeds and AI-powered content insights"
      />
    </Box>
  );
}
