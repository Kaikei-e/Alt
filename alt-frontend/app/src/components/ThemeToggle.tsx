"use client";

import React from "react";
import {
  Button,
  Text,
  VStack,
  useBreakpointValue,
  Box,
} from "@chakra-ui/react";
import { Sun, Moon } from "lucide-react";
import { useTheme } from "../hooks/useTheme";

export interface ThemeToggleProps {
  size?: "sm" | "md" | "lg";
  showLabel?: boolean;
  variant?: "minimal" | "glass" | string;
}

const THEME_ICONS = {
  vaporwave: Moon,
  "liquid-beige": Sun,
};

export const ThemeToggle: React.FC<ThemeToggleProps> = ({
  size = "md",
  showLabel = false,
}) => {
  const { currentTheme, toggleTheme } = useTheme();

  // Wait for the theme to be resolved before rendering
  const [mounted, setMounted] = React.useState(false);

  // Always call useEffect in the same order - fix for hooks order error
  React.useEffect(() => setMounted(true), []);

  const currentSize =
    useBreakpointValue({
      base: size === "lg" ? "md" : size,
      md: size,
    }) || size;

  const lastToggleRef = React.useRef<number>(0);

  const handleToggle = () => {
    const now = Date.now();
    const elapsed = now - lastToggleRef.current;

    // Ignore the interaction if it occurs within the debounce window.
    // We still update the timestamp so that rapidly repeated clicks keep
    // extending the window and only the very first one is honoured.
    if (elapsed < 250) {
      lastToggleRef.current = now;
      return;
    }

    lastToggleRef.current = now;
    toggleTheme();
  };

  const handleKeyDown = (event: React.KeyboardEvent) => {
    const { key, code } = event;
    if (
      key === " " ||
      key === "Spacebar" ||
      code === "Space" ||
      key === "Enter"
    ) {
      event.preventDefault();
      handleToggle();
    }
  };

  const sizeMap = {
    sm: { icon: "16px", button: "32px", fontSize: "xs" },
    md: { icon: "20px", button: "40px", fontSize: "sm" },
    lg: { icon: "24px", button: "48px", fontSize: "md" },
  };

  const currentStyles = sizeMap[currentSize];

  if (!mounted) {
    // Render a placeholder or nothing until the theme is resolved to prevent hydration mismatch
    return <Box w={currentStyles.button} h={currentStyles.button} />;
  }

  const IconComponent = THEME_ICONS[currentTheme];
  const nextTheme =
    currentTheme === "liquid-beige" ? "vaporwave" : "liquid-beige";
  const nextThemeLabel =
    nextTheme === "vaporwave" ? "Vaporwave" : "Liquid Beige";
  const currentThemeLabel =
    currentTheme === "vaporwave" ? "Vaporwave" : "Liquid Beige";

  // Use CSS variables from global.css that work with data-style attribute
  const buttonStyles = {
    width: currentStyles.button,
    height: currentStyles.button,
    minWidth: currentStyles.button,
    borderRadius: currentSize === "sm" ? "8px" : "12px",
    position: "relative" as const,
    overflow: "hidden" as const,
    transition: "all 0.2s cubic-bezier(0.4, 0, 0.2, 1)",
    // Use CSS variables that are defined in global.css
    bg: "var(--surface-bg)",
    backdropFilter: "blur(var(--surface-blur))",
    border: "1px solid var(--surface-border)",
    _hover: {
      borderColor: "var(--accent-primary)",
      transform: "translateY(-1px)",
      boxShadow: "0 4px 12px rgba(255, 0, 110, 0.2)",
    },
    _focus: {
      outline: "none",
      borderColor: "var(--accent-primary)",
      boxShadow: "0 0 0 2px var(--accent-primary)",
    },
    _active: {
      transform: "translateY(0px)",
    },
  };

  const iconStyles = {
    color: "var(--accent-primary)",
    fontSize: currentStyles.icon,
  };

  const buttonContent = IconComponent ? (
    <IconComponent style={iconStyles} />
  ) : null;

  if (showLabel) {
    return (
      <VStack gap={2} align="center" data-testid="theme-toggle">
        <Button
          onClick={handleToggle}
          onKeyDown={handleKeyDown}
          role="switch"
          aria-checked={currentTheme === "vaporwave"}
          aria-label={`Switch to ${nextThemeLabel} theme. Current theme: ${currentThemeLabel}`}
          data-testid="theme-toggle-button"
          css={buttonStyles}
        >
          {buttonContent}
        </Button>
        <Text
          fontSize={currentStyles.fontSize}
          color="var(--accent-secondary)"
          fontWeight="medium"
          textAlign="center"
          data-testid="theme-toggle-label"
        >
          {currentThemeLabel}
        </Text>
      </VStack>
    );
  }

  return (
    <Box data-testid="theme-toggle" display="inline-block">
      <Button
        onClick={handleToggle}
        onKeyDown={handleKeyDown}
        role="switch"
        aria-checked={currentTheme === "vaporwave"}
        aria-label={`Switch to ${nextThemeLabel} theme. Current theme: ${currentThemeLabel}`}
        data-testid="theme-toggle-button"
        css={buttonStyles}
      >
        {buttonContent}
      </Button>
    </Box>
  );
};

export default ThemeToggle;
