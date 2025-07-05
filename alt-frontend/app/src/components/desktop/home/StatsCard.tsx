"use client";

import React from "react";
import { Box, HStack, Text, Icon } from "@chakra-ui/react";
import { AnimatedNumber } from "@/components/mobile/stats/AnimatedNumber";
import Loading from "@/components/mobile/utils/Loading";

interface StatsCardProps {
  icon: React.ComponentType<{ size?: number }>;
  label: string;
  value: number;
  trend?: string;
  trendLabel?: string;
  color: 'primary' | 'secondary' | 'tertiary';
  isLoading?: boolean;
}

export const StatsCard: React.FC<StatsCardProps> = ({
  icon,
  label,
  value,
  trend,
  trendLabel,
  color,
  isLoading = false,
}) => {
  const getColorVariable = (colorType: 'primary' | 'secondary' | 'tertiary') => {
    switch (colorType) {
      case 'primary':
        return 'var(--alt-primary)';
      case 'secondary':
        return 'var(--alt-secondary)';
      case 'tertiary':
        return 'var(--alt-tertiary)';
      default:
        return 'var(--alt-primary)';
    }
  };

  const colorVar = getColorVariable(color);

  return (
    <Box
      className="glass"
      p={6}
      borderRadius="var(--radius-xl)"
      border="1px solid var(--surface-border)"
      _hover={{
        transform: "translateY(-2px)",
        boxShadow: `0 8px 32px ${colorVar}20`,
      }}
      transition="all var(--transition-speed) ease"
      h="auto"
    >
      <HStack justify="space-between" mb={4}>
        <Icon as={icon} color={colorVar} boxSize={6} />
        <Box
          bg={colorVar}
          borderRadius="full"
          p={1}
          opacity={0.1}
        />
      </HStack>
      
      {isLoading ? (
        <Loading isLoading={isLoading} />
      ) : (
        <AnimatedNumber
          value={value}
          duration={1000}
          textProps={{
            fontSize: "2xl",
            fontWeight: "bold",
            color: "var(--text-primary)",
          }}
        />
      )}
      
      <Text fontSize="sm" color="var(--text-muted)" mt={1}>
        {label}
      </Text>
      
      {trend && (
        <Text fontSize="xs" color={colorVar} mt={2}>
          {trend} {trendLabel && trendLabel}
        </Text>
      )}
    </Box>
  );
};

export default StatsCard;