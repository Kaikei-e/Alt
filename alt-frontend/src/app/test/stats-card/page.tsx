"use client";

import { Box, Text } from "@chakra-ui/react";
import { TrendingUp } from "lucide-react";
import StatsCard from "@/components/desktop/home/StatsCard";

export default function StatsCardTest() {
  return (
    <Box p={8} bg="var(--app-bg)" minH="100vh">
      <Text fontSize="2xl" mb={6} textAlign="center">
        StatsCard Component Test
      </Text>
      <Box maxW="300px" mx="auto">
        <StatsCard
          icon={TrendingUp}
          label="Weekly Reads"
          value={156}
          trend="+12%"
          trendLabel="from last week"
          color="primary"
        />
      </Box>
    </Box>
  );
}
