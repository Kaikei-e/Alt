import { Button, Flex, Spinner, Text } from "@chakra-ui/react";
import { Feed } from "@/schema/feed";
import Link from "next/link";
import { feedsApi } from "@/lib/api";
import { useState } from "react";

type FeedCardProps = {
  feed: Feed;
  isReadStatus: boolean;
  setIsReadStatus: (isReadStatus: boolean) => void;
};

export default function FeedCard({
  feed,
  isReadStatus,
  setIsReadStatus,
}: FeedCardProps) {
  const [isLoading, setIsLoading] = useState(false);

  const handleReadStatus = async (url: string) => {
    try {
      setIsLoading(true);
      await feedsApi.updateFeedReadStatus(url);
      setIsReadStatus(true);
      setIsLoading(false);
    } catch (error) {
      console.error("Error updating feed read status", error);
    }
  };

  if (isLoading) {
    return <Spinner size="md" color="black" />;
  }

  if (isReadStatus) {
    return null;
  }

  return (
    <Flex
      key={feed.id}
      flexDirection="column"
      justifyContent="center"
      alignItems="center"
      width="100%"
      bg="blue.100"
      borderRadius="2xl"
      p={3}
    >
      <Text fontSize="md" fontWeight="bold" color="gray.500">
        <Link href={feed.link} target="_blank">
          {feed.title}
        </Link>
      </Text>
      <Text fontSize="xs" color="gray.500">
        {feed.description.slice(0, 300)}...
      </Text>

      <Flex
        flexDirection="row"
        justifyContent="space-between"
        w="100%"
        alignItems="center"
      >
        <Button
          onClick={() => handleReadStatus(feed.link)}
          colorScheme="green"
          size="sm"
          mt={2}
          w="50%"
          disabled={isLoading}
        >
          Mark as read
        </Button>
        <Text fontSize="xs" color="gray.500" ml={2} mr={2} textAlign="center">
          {feed.published}
        </Text>
      </Flex>
    </Flex>
  );
}
