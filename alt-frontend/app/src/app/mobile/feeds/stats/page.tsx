"use client";

import { FeedStatsSummary } from "@/schema/feedStats";
import { feedsApiSse } from "@/lib/apiSse";
import { Flex, Text, Box, Card, SimpleGrid } from "@chakra-ui/react";
import { FloatingMenu } from "@/components/mobile/utils/FloatingMenu";
import { useEffect, useState, useRef } from "react";

export default function FeedsStatsPage() {
  const [feedAmount, setFeedAmount] = useState(0);
  const [unsummarizedArticlesAmount, setUnsummarizedArticlesAmount] = useState(0);
  const eventSourceRef = useRef<{
    close: () => void;
    getReadyState: () => number;
  } | null>(null);

  useEffect(() => {
    const sseConnection = feedsApiSse.getFeedsStats(
      (data: FeedStatsSummary) => {
        if (data.feed_amount?.amount !== undefined) {
          setFeedAmount(data.feed_amount.amount);
        }
        if (data.summarized_feed?.amount !== undefined) {
          setUnsummarizedArticlesAmount(data.summarized_feed.amount);
        }
      },
      (event) => {
        console.error("SSE connection error:", event);
      },
    );

    eventSourceRef.current = sseConnection;

    return () => {
      eventSourceRef.current?.close();
    };
  }, []);

  return (
    <Box>
      <Flex flexDirection="column" gap="4" p="4">
        <Text fontSize="2xl" fontWeight="bold" mb="4">
          Feeds Statistics
        </Text>

        <SimpleGrid columns={1} gap="4">
          <Card.Root>
            <Card.Body>
              <Text fontSize="lg" fontWeight="semibold">
                Feeds: {feedAmount}
              </Text>
            </Card.Body>
          </Card.Root>

          <Card.Root>
            <Card.Body>
              <Text fontSize="lg" fontWeight="semibold">
                Unsummarized Articles: {unsummarizedArticlesAmount}
              </Text>
            </Card.Body>
          </Card.Root>
        </SimpleGrid>
      </Flex>
      <FloatingMenu />
    </Box>
  );
}
