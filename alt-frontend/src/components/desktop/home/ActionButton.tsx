"use client";

import { Button, Icon, Text } from "@chakra-ui/react";
import NextLink from "next/link";
import type React from "react";

interface ActionButtonProps {
  icon: React.ComponentType<{ size?: number }>;
  label: string;
  href: string;
  prefetch?: boolean;
}

export const ActionButton: React.FC<ActionButtonProps> = ({
  icon,
  label,
  href,
  prefetch = true,
}) => {
  return (
    <NextLink href={href} prefetch={prefetch} style={{ width: "100%" }}>
      <Button
        data-testid="action-button"
        h="90px"
        w="full"
        minH="90px"
        maxH="90px"
        minW="120px"
        bg="var(--surface-bg)"
        border="1px solid var(--surface-border)"
        borderRadius="var(--radius-lg)"
        backdropFilter="blur(var(--surface-blur))"
        display="flex"
        flexDirection="column"
        alignItems="center"
        justifyContent="center"
        gap={2}
        color="var(--text-primary)"
        p={3}
        _hover={{
          transform: "translateY(-2px)",
          borderColor: "var(--alt-primary)",
          boxShadow: "0 8px 24px rgba(0, 0, 0, 0.15)",
          bg: "var(--surface-hover)",
        }}
        transition="all var(--transition-speed) ease"
      >
        <Icon
          as={icon}
          color="var(--alt-primary)"
          boxSize={5}
          aria-hidden="true"
        />
        <Text
          fontSize="sm"
          fontWeight="medium"
          color="var(--text-primary)"
          textAlign="center"
          lineHeight="1.2"
        >
          {label}
        </Text>
      </Button>
    </NextLink>
  );
};

export default ActionButton;
