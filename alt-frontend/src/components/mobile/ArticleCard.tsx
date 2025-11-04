import { Box, Button, Dialog, Flex, Portal, Text } from "@chakra-ui/react";
import type { Article } from "@/schema/article";

interface ArticleCardProps {
  article: Article;
}

export const ArticleCard = ({ article }: ArticleCardProps) => {
  return (
    <Box
      p="2px"
      borderRadius="18px"
      border="2px solid var(--surface-border)"
      transition="transform 0.3s ease-in-out, box-shadow 0.3s ease-in-out"
      cursor="pointer"
      data-testid="article-card"
    >
      <Box
        className="glass"
        w="full"
        p={5}
        borderRadius="16px"
        mb={4}
        role="article"
        aria-label={`Article: ${article.title}`}
      >
        <Flex direction="column" gap={3}>
          <Text
            fontSize="lg"
            fontWeight="semibold"
            color="var(--accent-primary)"
            lineHeight="1.4"
            overflow="hidden"
            textOverflow="ellipsis"
            display="-webkit-box"
            style={{
              WebkitLineClamp: 2,
              WebkitBoxOrient: "vertical",
            }}
          >
            {article.title}
          </Text>

          <Dialog.Root size="cover" placement="center" motionPreset="slide-in-bottom">
            <Dialog.Trigger asChild>
              <Button
                size="sm"
                borderRadius="full"
                bg="var(--alt-primary)"
                color="var(--text-primary)"
                fontWeight="bold"
                px={4}
                minHeight="36px"
                minWidth="100px"
                fontSize="sm"
                border="1px solid rgba(255, 255, 255, 0.2)"
                _hover={{
                  bg: "var(--accent-gradient)",
                  transform: "scale(1.05)",
                  boxShadow: "0 4px 12px var(--accent-primary)",
                }}
                _active={{ transform: "scale(0.98)" }}
                transition="all 0.2s ease"
              >
                Details
              </Button>
            </Dialog.Trigger>
            <Portal>
              <Dialog.Backdrop bg="blackAlpha.800" />
              <Dialog.Positioner>
                <Dialog.Content
                  className="glass"
                  backdropFilter="blur(20px)"
                  border="1px solid"
                  borderColor="var(--surface-border)"
                  borderRadius="20px"
                  boxShadow="0 25px 50px var(--accent-primary)"
                  mx={4}
                  my={8}
                  maxH="70vh"
                >
                  <Dialog.Header>
                    <Dialog.Title
                      textAlign="center"
                      color="var(--alt-primary)"
                      fontSize="xl"
                      fontWeight="bold"
                      pr={8}
                    >
                      {article.title}
                    </Dialog.Title>
                  </Dialog.Header>
                  <Dialog.Body px={6} py={4} maxH="60vh" overflowY="auto">
                    <Text color="var(--text-primary)" lineHeight="1.6" fontSize="md">
                      {article.content || "No additional content available for this article."}
                    </Text>
                  </Dialog.Body>
                  <Dialog.Footer px={6} py={4}>
                    <Dialog.ActionTrigger asChild>
                      <Button
                        variant="outline"
                        color="var(--alt-primary)"
                        borderRadius="full"
                        size="md"
                        w="full"
                        border="1px solid var(--alt-primary)"
                        _hover={{
                          bg: "var(--accent-gradient)",
                          color: "var(--text-primary)",
                        }}
                      >
                        Close
                      </Button>
                    </Dialog.ActionTrigger>
                  </Dialog.Footer>
                </Dialog.Content>
              </Dialog.Positioner>
            </Portal>
          </Dialog.Root>
        </Flex>
      </Box>
    </Box>
  );
};
