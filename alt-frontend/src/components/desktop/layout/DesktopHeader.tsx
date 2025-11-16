"use client";

import { Box, Flex, Heading, Input } from "@chakra-ui/react";
import { Search } from "lucide-react";
import type React from "react";
import { ThemeToggle } from "@/components/ThemeToggle";
import type { DesktopHeaderProps } from "@/types/desktop-feeds";

export const DesktopHeader: React.FC<DesktopHeaderProps> = ({
  searchQuery = "",
  onSearchChange,
}) => {
  return (
    <Flex
      justify="space-between"
      align="center"
      p={{ base: 4, md: 6 }}
      h="80px"
      data-testid="desktop-header"
    >
      {/* 左側：ロゴと統計 */}
      <Flex align="center" gap={6}>
        <Heading
          size="lg"
          color="var(--text-primary)"
          fontFamily="var(--font-display)"
        >
          Alt / Feeds
        </Heading>
      </Flex>

      {/* 中央：検索バー */}
      <Flex flex={1} justify="center" maxW="400px" mx={8}>
        <Box position="relative" w="100%">
          <Box
            position="absolute"
            left={3}
            top="50%"
            transform="translateY(-50%)"
            pointerEvents="none"
            zIndex={1}
          >
            <Search color="var(--text-muted)" size={18} />
          </Box>
          <Input
            placeholder="Search feeds..."
            value={searchQuery}
            className="glass"
            border="1px solid var(--surface-border)"
            bg="var(--surface-bg)"
            color="var(--text-primary)"
            _placeholder={{ color: "var(--text-muted)" }}
            _focus={{
              borderColor: "var(--accent-primary)",
              boxShadow: "0 0 0 1px var(--accent-primary)",
            }}
            borderRadius="var(--radius-full)"
            pl={10}
            onChange={(e) => onSearchChange(e.target.value)}
          />
        </Box>
      </Flex>

      {/* 右側：テーマトグル */}
      <Flex align="center">
        <ThemeToggle size="md" showLabel={false} />
      </Flex>
    </Flex>
  );
};
