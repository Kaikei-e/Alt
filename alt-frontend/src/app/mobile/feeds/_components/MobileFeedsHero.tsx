import { Box, Heading, VStack } from "@chakra-ui/react";
import { HeroTip } from "@/components/mobile/hero/HeroTip";

/**
 * Mobile Feeds Hero component.
 * Server Component that renders the hero section for mobile feeds pages.
 * Designed to be rendered early in the page to optimize LCP.
 *
 * This component is isolated from the feed list to ensure it becomes the LCP element.
 */
export function MobileFeedsHero() {
  return (
    <Box
      as="header"
      px={5}
      pt={4}
      pb={2}
      minH="120px"
      width="100%"
      data-lcp-hero="header"
    >
      <VStack align="stretch" gap={3}>
        <Heading as="h3" size="xl" color="var(--text-primary)" fontWeight={600}>
          Your Feeds
        </Heading>
        <HeroTip />
      </VStack>
    </Box>
  );
}
