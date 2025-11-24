import { Box, Text } from "@chakra-ui/react";

/**
 * Hero Tip component for LCP optimization.
 * This component is designed to be the Largest Contentful Paint (LCP) element
 * on mobile feeds pages. It uses static text and system fonts to minimize
 * render delay and ensure consistent LCP timing.
 */
export function HeroTip() {
  return (
    <Box
      as="section"
      data-lcp-hero="tip"
      aria-label="Tip"
      className="lcp-hero-tip"
    >
      <Text as="p">
        Tip: You can swipe through your feeds and manage them here.
      </Text>
    </Box>
  );
}

