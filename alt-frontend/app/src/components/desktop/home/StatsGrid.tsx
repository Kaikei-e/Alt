"use client";

import React from "react";
import { Grid, Box, Text } from "@chakra-ui/react";
import { Rss, FileText, Zap } from "lucide-react";
import { StatsCard } from "./StatsCard";

interface StatsGridProps {
  stats: Array<{
    id: string;
    icon: React.ComponentType<{ size?: number }>;
    label: string;
    value: number;
    trend?: string;
    trendLabel?: string;
    color: "primary" | "secondary" | "tertiary";
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
      {stats.length > 0
        ? stats.map((stat) => (
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
          ))
        : // Show loading states or default cards when no stats available
          Array.from({ length: 3 }, (_, index) => (
            <StatsCard
              key={`loading-${index}`}
              icon={index === 0 ? Rss : index === 1 ? FileText : Zap}
              label={
                index === 0
                  ? "Total Feeds"
                  : index === 1
                    ? "AI Processed"
                    : "Unread Articles"
              }
              value={0}
              trend={index === 0 ? "+12%" : index === 1 ? "+89%" : "+5%"}
              trendLabel={
                index === 0
                  ? "from last week"
                  : index === 1
                    ? "efficiency boost"
                    : "new today"
              }
              color={
                index === 0 ? "primary" : index === 1 ? "secondary" : "tertiary"
              }
              isLoading={isLoading}
            />
          ))}
    </Grid>
  );
};

export default StatsGrid;
