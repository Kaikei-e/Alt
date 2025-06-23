"use client";

import { Box, Flex, Text, Icon } from "@chakra-ui/react";
import { IconType } from "react-icons";

interface StatCardProps {
  /** The label/title for the statistic */
  label: string;
  /** The numeric value to display */
  value: number;
  /** Icon component */
  icon: IconType;
  /** Additional description text */
  description: string;
}

export const StatCard = ({ icon, label, value, description }: StatCardProps) => {
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
    >
      <Flex direction="column" gap={3}>
        <Flex align="center" gap={2}>
          <Icon as={icon} color="#ff006e" boxSize={5} />
          <Text
            fontSize="sm"
            textTransform="uppercase"
            color="whiteAlpha.600"
            letterSpacing="wider"
          >
            {label}
          </Text>
        </Flex>

        <Text fontSize="3xl" fontWeight="bold" color="white">
          {value.toLocaleString()}
        </Text>

        <Text fontSize="sm" color="whiteAlpha.700" lineHeight="1.5">
          {description}
        </Text>
      </Flex>
    </Box>
  );
};

export default StatCard;