import { Flex, Text } from "@chakra-ui/react";
import { Feed } from "@/schema/feed";
import Link from "next/link";

export default function FeedCard({ feed }: { feed: Feed }) {
  return (
    <Flex
      key={feed.id}
      flexDirection="column"
      justifyContent="center"
      alignItems="center"
      height="1/12"
      width="90%"
      bg="gray.100"
      borderRadius="md"
    >
      <Text fontSize="lg" fontWeight="bold">
        <Link href={feed.link} color="gray.500" target="_blank">
          {feed.title}
        </Link>
      </Text>
      <Text fontSize="sm" color="gray.500">
        {feed.description}
      </Text>
      <Text fontSize="sm" color="gray.500">
        {feed.pubDate}
      </Text>
    </Flex>
  );
}
