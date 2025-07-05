"use client";

import React from "react";
import { Box, Grid, VStack } from "@chakra-ui/react";
import { ActionButton } from "./ActionButton";
import { StatsFooter } from "./StatsFooter";

interface QuickActionsPanelProps {
  actions: Array<{
    id: number;
    label: string;
    icon: React.ComponentType<{ size?: number }>;
    href: string;
  }>;
  additionalStats?: {
    weeklyReads: number;
    aiProcessed: number;
    bookmarks: number;
  };
}

export const QuickActionsPanel: React.FC<QuickActionsPanelProps> = ({
  actions,
  additionalStats,
}) => {
  return (
    <Box
      data-testid="quick-actions-panel"
      className="glass"
      p={6}
      borderRadius="var(--radius-xl)"
      border="1px solid var(--surface-border)"
      h="400px"
      display="flex"
      flexDirection="column"
    >
      <VStack gap={4} flex="1">
        <Grid
          data-testid="actions-grid"
          templateColumns="repeat(2, 1fr)"
          gap={4}
          w="full"
          flex="1"
        >
          {actions.map((action) => (
            <ActionButton
              key={action.id}
              icon={action.icon}
              label={action.label}
              href={action.href}
            />
          ))}
        </Grid>

        {additionalStats && (
          <StatsFooter stats={additionalStats} />
        )}
      </VStack>
    </Box>
  );
};

export default QuickActionsPanel;