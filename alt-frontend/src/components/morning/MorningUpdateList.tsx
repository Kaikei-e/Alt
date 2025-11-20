"use client";

import { Box, Flex, Text } from "@chakra-ui/react";
import { MorningUpdateItem } from "./MorningUpdateItem";
import type { MorningUpdate } from "@/schema/morning";

type MorningUpdateListProps = {
  updates: MorningUpdate[];
};

export const MorningUpdateList = ({ updates }: MorningUpdateListProps) => {
  if (updates.length === 0) {
    return (
      <Box
        textAlign="center"
        py={8}
        color="var(--text-secondary)"
        data-testid="morning-update-list-empty"
      >
        <Text fontSize="sm">No overnight updates available</Text>
      </Box>
    );
  }

  return (
    <Flex direction="column" gap={2} data-testid="morning-update-list">
      {updates.map((update) => (
        <MorningUpdateItem key={update.group_id} update={update} />
      ))}
    </Flex>
  );
};

