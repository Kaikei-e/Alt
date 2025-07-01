import { Article } from "@/schema/article";
import { Box, Text, Flex, Button, Dialog, Portal } from "@chakra-ui/react";

interface ArticleCardProps {
  article: Article;
}

export const ArticleCard = ({ article }: ArticleCardProps) => {
  return (
    <Box
      data-testid="article-card"
      className="article-card-wrapper"
      width="100%"
    >
      <Box
        className="glass"
        w="full"
        p={5}
        mb={4}
        borderRadius="18px"
        cursor="pointer"
        transition="all 0.3s ease"
        _hover={{
          transform: "translateY(-5px)",
          boxShadow: "0 20px 40px var(--accent-primary)",
        }}
      >
        <Flex direction="column" gap={3}>
          <Text
            fontSize="lg"
            fontWeight="bold"
            color="white"
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

          <Dialog.Root
            size="cover"
            placement="center"
            motionPreset="slide-in-bottom"
          >
            <Dialog.Trigger asChild>
              <Button
                color="var(--alt-primary)"
                size="sm"
                borderRadius="10px"
                _hover={{
                  transform: "scale(1.05)",
                }}
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
                  borderRadius="20px"
                  boxShadow="0 25px 50px var(--accent-primary)"
                  mx={4}
                  my={8}
                  maxH="90vh"
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
                    <Text
                      color="var(--text-primary)"
                      lineHeight="1.6"
                      fontSize="md"
                    >
                      {article.content ||
                        "No additional content available for this article."}
                    </Text>
                  </Dialog.Body>
                  <Dialog.Footer px={6} py={4}>
                    <Dialog.ActionTrigger asChild>
                      <Button
                        variant="outline"
                        color="var(--alt-primary)"
                        borderRadius="10px"
                        size="md"
                        w="full"
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
