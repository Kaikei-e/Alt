"use client";

import { Box, Text } from "@chakra-ui/react";
import { Plus } from "lucide-react";
import ActionButton from "@/components/desktop/home/ActionButton";

export default function ActionButtonTest() {
  return (
    <Box p={8} bg="var(--app-bg)" minH="100vh">
      <Text fontSize="2xl" mb={6} textAlign="center">
        ActionButton Component Test
      </Text>
      <Box display="flex" justifyContent="center">
        <ActionButton
          label="Add Feed"
          icon={Plus}
          href="/desktop/feeds/register"
        />
      </Box>
    </Box>
  );
}
