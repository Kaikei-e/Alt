"use client";

import React from "react";
import { Grid, Box, Text } from "@chakra-ui/react";
import { StatsCard } from "./StatsCard";

interface StatsGridProps {
  stats: Array<{
    id: string;
    icon: React.ComponentType<{ size?: number }>;
    label: string;
    value: number;
    trend?: string;
    trendLabel?: string;
    color: 'primary' | 'secondary' | 'tertiary';
  }>;
  isLoading?: boolean;
  error?: string | null;
}

export const StatsGrid: React.FC<StatsGridProps> = ({
  stats,
  isLoading = false,
  error = null,
}) => {
  if (error) {
    return (
      <Box
        data-testid="error-message"
        className="glass"
        p={6}
        borderRadius="var(--radius-xl)"
        border="1px solid var(--surface-border)"
        textAlign="center"
      >
        <Text color="var(--text-muted)">Failed to load statistics</Text>
      </Box>
    );
  }

  return (
    <Grid
      data-testid="stats-grid"
      templateColumns="repeat(3, 1fr)"
      gap={6}
      w="full"
    >
      {stats.map((stat) => (
        <StatsCard
          key={stat.id}
          icon={stat.icon}
          label={stat.label}
          value={stat.value}
          trend={stat.trend}
          trendLabel={stat.trendLabel}
          color={stat.color}
          isLoading={isLoading}
        />
      ))}
    </Grid>
  );
};

export default StatsGrid;