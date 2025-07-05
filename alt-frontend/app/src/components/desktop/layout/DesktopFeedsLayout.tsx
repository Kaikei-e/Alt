'use client';

import React from 'react';
import { Grid, GridItem } from '@chakra-ui/react';
import { DesktopFeedsLayoutProps } from '@/types/desktop-feeds';
import { RightPanel } from '@/components/desktop/analytics/RightPanel';

export const DesktopFeedsLayout: React.FC<DesktopFeedsLayoutProps> = ({
  children,
  sidebar,
  header
}) => {
  return (
    <Grid
      templateRows="auto 1fr"
      minH="100vh"
      bg="var(--app-bg)"
      data-testid="desktop-layout"
    >
      {/* ヘッダー */}
      <GridItem
        position="sticky"
        top={0}
        zIndex={100}
        className="glass"
        borderBottom="1px solid var(--surface-border)"
      >
        {header}
      </GridItem>

      {/* メインコンテンツ */}
      <GridItem>
        <Grid
          templateColumns={{
            base: "1fr",
            md: "240px 1fr",
            lg: "280px 1fr 320px",
            xl: "300px 1fr 340px"
          }}
          gap={{ base: 4, md: 6, lg: 8 }}
          p={{ base: 4, md: 6, lg: 8 }}
          h="calc(100vh - 80px)"
          maxW="none"
          mx={0}
        >
          {/* サイドバー */}
          <GridItem
            position="sticky"
            top="120px"
            h="fit-content"
            maxH="calc(100vh - 140px)"
            overflowY="auto"
            display={{ base: "none", md: "block" }}
          >
            {sidebar}
          </GridItem>

          {/* タイムライン */}
          <GridItem 
            display="flex" 
            alignItems="stretch" 
            px={{ base: 4, md: 6, lg: 8 }}
            data-testid="main-content"
          >
            {children}
          </GridItem>

          {/* 右パネル（Analytics） */}
          <GridItem
            position="sticky"
            top="120px"
            h="fit-content"
            maxH="calc(100vh - 140px)"
            overflowY="auto"
            display={{ base: "none", lg: "block" }}
          >
            <RightPanel />
          </GridItem>
        </Grid>
      </GridItem>
    </Grid>
  );
};