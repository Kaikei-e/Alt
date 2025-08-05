"use client";

import React from "react";
import { Box, Grid, GridItem } from "@chakra-ui/react";
import { DesktopFeedsLayoutProps } from "@/types/desktop-feeds";
import { RightPanel } from "@/components/desktop/analytics/RightPanel";
import { ThemeToggle } from "@/components/ThemeToggle";

export const DesktopFeedsLayout: React.FC<DesktopFeedsLayoutProps> = ({
  children,
  sidebar,
}) => {
  return (
    <Box
      minH="100vh"
      bg="var(--app-bg)"
      data-testid="desktop-layout"
      position="relative"
    >
      {/* Theme Toggle */}
      <Box position="fixed" top={4} right={4} zIndex={1000}>
        <ThemeToggle size="md" />
      </Box>

      <Grid templateRows="auto 1fr" maxH="100vh" overflow="hidden">
        {/* メインコンテンツ */}
        <GridItem overflow="hidden">
          <Grid
            templateColumns={{
              base: "1fr",
              md: "240px 1fr",
              lg: "260px 1fr 300px",
              xl: "280px 1fr 320px",
            }}
            gap={{ base: 4, md: 4, lg: 6 }}
            p={{ base: 4, md: 4, lg: 6 }}
            h="100%"
            maxW="none"
            mx={0}
          >
            {/* サイドバー */}
            <GridItem
              overflowY="auto"
              overflowX="hidden"
              display={{ base: "none", md: "block" }}
            >
              {sidebar}
            </GridItem>

            {/* タイムライン */}
            <GridItem
              display="flex"
              alignItems="stretch"
              bg="var(--app-bg)"
              px={{ base: 2, md: 3, lg: 4 }}
              overflow="hidden"
              data-testid="main-content"
            >
              {children}
            </GridItem>

            {/* 右パネル（Analytics） */}
            <GridItem
              overflowY="auto"
              overflowX="hidden"
              display={{ base: "none", lg: "block" }}
            >
              <RightPanel />
            </GridItem>
          </Grid>
        </GridItem>
      </Grid>
    </Box>
  );
};
