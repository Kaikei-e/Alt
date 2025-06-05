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
      height="3/12"
      width="100%"
      bg="blue.100"
      borderRadius="2xl"
      p={4}
    >
      <Text fontSize="md" fontWeight="bold" color="gray.500">
        <Link href={feed.link} target="_blank">
          {feed.title}
        </Link>
      </Text>
      <Text fontSize="xs" color="gray.500">
        {feed.description.slice(0, 200)}...
      </Text>
      <Text fontSize="xs" color="gray.500">
        {feed.published}
      </Text>
    </Flex>
  );
}
