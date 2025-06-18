"use client";

import { FeedStatsSummary } from "@/schema/feedStats";
import { feedsApiSse } from "@/lib/apiSse";
import { Flex, Text, Box } from "@chakra-ui/react";
import { FloatingMenu } from "@/components/mobile/utils/FloatingMenu";
import { useEffect, useState, useRef } from "react";

export default function FeedsStatsPage() {
  const [feedAmount, setFeedAmount] = useState(0);
  const [summarizedFeedAmount, setSummarizedFeedAmount] = useState(0);
  const eventSourceRef = useRef<{ close: () => void; getReadyState: () => number } | null>(null);

  useEffect(() => {
    const sseConnection = feedsApiSse.getFeedsStats(
      (data: FeedStatsSummary) => {
        if (data.feed_amount?.amount !== undefined) {
          setFeedAmount(data.feed_amount.amount);
        }
        if (data.summarized_feed?.amount !== undefined) {
          setSummarizedFeedAmount(data.summarized_feed.amount);
        }
      },
      (event) => {
        console.error("SSE connection error:", event);
      }
    );

    eventSourceRef.current = sseConnection;

    return () => {
      eventSourceRef.current?.close();
    };
  }, []);

  return (
    <Box>
      <Flex flexDirection="column" gap="2" p="4">
        <Text fontSize="2xl" fontWeight="bold">
          Feeds Stats
        </Text>
        <Text>Feeds: {feedAmount}</Text>
        <Text>Summarized Feeds: {summarizedFeedAmount}</Text>
      </Flex>
      <FloatingMenu />
    </Box>
  );
}
