"use client";

import { Box, Flex, Icon, Text } from "@chakra-ui/react";
import type { ElementType } from "react";

// Safe number formatter with fallbacks
const safeFormatNumber = (value: number): string => {
  if (value > Number.MAX_SAFE_INTEGER) {
    return "∞";
  }
  if (value < 0) {
    return "0";
  }
  return value.toLocaleString();
};

interface StatCardProps {
  /** The label/title for the statistic */
  label: string;
  /** The numeric value to display */
  value: number;
  /** Icon component */
  icon: ElementType;
  /** Additional description text */
  description: string;
  /** Optional test ID for testing */
  "data-testid"?: string;
}

export const StatCard = ({
  icon,
  label,
  value,
  description,
  "data-testid": testId,
}: StatCardProps) => {
  return (
    <Box
      className="glass"
      w="full"
      p={6}
      borderRadius="18px"
      cursor="pointer"
      transition="all 0.3s ease"
      _hover={{
        transform: "translateY(-5px)",
        boxShadow: "0 20px 40px rgba(255, 0, 110, 0.3)",
      }}
      data-testid={testId}
    >
      <Flex direction="column" gap={3}>
        <Flex align="center" gap={2}>
          <Icon as={icon} color="var(--alt-primary)" boxSize={5} />
          <Text
            fontSize="sm"
            textTransform="uppercase"
            color="var(--text-primary)"
            letterSpacing="wider"
          >
            {label}
          </Text>
        </Flex>

        <Text fontSize="3xl" fontWeight="bold" color="var(--text-primary)">
          {safeFormatNumber(value)}
        </Text>

        <Text fontSize="sm" color="var(--text-primary)" lineHeight="1.5">
          {description}
        </Text>
      </Flex>
    </Box>
  );
};

export default StatCard;
