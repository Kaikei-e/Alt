"use client";

import { Box, BoxProps } from "@chakra-ui/react";
import { ReactNode } from "react";

interface GlassPanelProps extends Omit<BoxProps, 'children'> {
  children: ReactNode;
  /** Enable gradient border effect */
  gradientBorder?: boolean;
  /** Intensity of the glassmorphism effect (1-10) */
  glassIntensity?: number;
  /** Enable hover effects */
  enableHover?: boolean;
}

export function GlassPanel({
  children,
  gradientBorder = false,
  glassIntensity = 5,
  enableHover = true,
  ...boxProps
}: GlassPanelProps) {
  // Calculate glass effect intensity
  const blurValue = Math.min(Math.max(glassIntensity, 1), 10) * 2; // 2-20px
  const opacityValue = 0.05 + (glassIntensity * 0.01); // 0.06-0.15

  const baseStyles = {
    background: `rgba(255, 255, 255, ${opacityValue})`,
    backdropFilter: `blur(${blurValue}px)`,
    border: "1px solid rgba(255, 255, 255, 0.2)",
    borderRadius: "1rem",
    position: "relative" as const,
    overflow: "hidden",
    transition: "all 0.2s ease",
    willChange: "transform",
  };

  const hoverStyles = enableHover ? {
    _hover: {
      transform: "translateY(-2px)",
      background: `rgba(255, 255, 255, ${opacityValue + 0.02})`,
      borderColor: "rgba(255, 255, 255, 0.3)",
      boxShadow: "0 8px 32px rgba(255, 0, 110, 0.15)",
    }
  } : {};

  const content = (
    <Box
      {...baseStyles}
      {...hoverStyles}
      {...boxProps}
    >
      {children}
    </Box>
  );

  // Wrap with gradient border if enabled
  if (gradientBorder) {
    return (
      <Box
        position="relative"
        p="2px"
        borderRadius="calc(1rem + 2px)"
        background="linear-gradient(45deg, #8338ec, #ff006e, #3a86ff)"
        _before={{
          content: '""',
          position: "absolute",
          top: 0,
          left: 0,
          right: 0,
          bottom: 0,
          borderRadius: "inherit",
          padding: "2px",
          background: "linear-gradient(45deg, #8338ec, #ff006e, #3a86ff)",
          mask: "linear-gradient(#fff 0 0) content-box, linear-gradient(#fff 0 0)",
          maskComposite: "exclude",
          WebkitMaskComposite: "xor",
        }}
      >
        {content}
      </Box>
    );
  }

  return content;
}

export default GlassPanel;