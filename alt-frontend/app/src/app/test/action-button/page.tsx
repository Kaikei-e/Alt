"use client";

import { Box } from "@chakra-ui/react";
import { Plus } from "lucide-react";
import { ActionButton } from "@/components/desktop/home/ActionButton";

export default function ActionButtonTestPage() {
  return (
    <Box p={8} minH="100vh" bg="var(--app-bg)">
      <ActionButton
        icon={Plus}
        label="Add Feed"
        href="/desktop/feeds"
      />
    </Box>
  );
}