"use client";

import {
  Box,
  Button,
  Dialog,
  Flex,
  Heading,
  Input,
  Portal,
  Spinner,
  Stack,
  Text,
  useDisclosure,
} from "@chakra-ui/react";
import { RefreshCw, Trash2 } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import * as v from "valibot";
import { ApiError } from "@/lib/api/core/ApiError";
import { feedApi } from "@/lib/api";
import { feedUrlSchema } from "@/schema/validation/feedUrlSchema";
import type { FeedLink } from "@/schema/feedLink";

type ActionMessage = {
  type: "success" | "error";
  text: string;
};

const validateUrl = (url: string): string | null => {
  if (!url.trim()) return "Please enter a feed URL";

  const result = v.safeParse(feedUrlSchema, { feed_url: url.trim() });
  if (!result.success) {
    return result.issues[0].message;
  }

  return null;
};

export default function ManageFeedsPage() {
  const [feedLinks, setFeedLinks] = useState<FeedLink[]>([]);
  const [isLoadingLinks, setIsLoadingLinks] = useState(true);
  const [loadingError, setLoadingError] = useState<string | null>(null);
  const [feedUrl, setFeedUrl] = useState("");
  const [validationError, setValidationError] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [selectedLink, setSelectedLink] = useState<FeedLink | null>(null);
  const [isDeleting, setIsDeleting] = useState(false);
  const [actionMessage, setActionMessage] = useState<ActionMessage | null>(
    null,
  );
  const { open, onOpen, onClose } = useDisclosure();
  const cancelRef = useRef<HTMLButtonElement>(null);

  const loadFeedLinks = useCallback(async () => {
    setIsLoadingLinks(true);
    setLoadingError(null);
    try {
      const links = await feedApi.listFeedLinks();
      setFeedLinks(links);
    } catch (error) {
      const message =
        error instanceof ApiError
          ? error.message
          : "Failed to load feed links.";
      setLoadingError(message);
    } finally {
      setIsLoadingLinks(false);
    }
  }, []);

  useEffect(() => {
    loadFeedLinks();
  }, [loadFeedLinks]);

  const sortedLinks = useMemo(
    () => [...feedLinks].sort((a, b) => a.url.localeCompare(b.url)),
    [feedLinks],
  );

  const resetForm = () => {
    setFeedUrl("");
    setValidationError(null);
  };

  const handleUrlChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    setFeedUrl(event.target.value);
    setValidationError(null);
    setActionMessage(null);
  };

  const handleSubmit = async (event: React.FormEvent) => {
    event.preventDefault();
    const error = validateUrl(feedUrl);
    if (error) {
      setValidationError(error);
      return;
    }

    setIsSubmitting(true);
    setActionMessage(null);

    try {
      await feedApi.registerRssFeed(feedUrl.trim());
      setActionMessage({
        type: "success",
        text: "Feed registered successfully.",
      });
      resetForm();
      await loadFeedLinks();
    } catch (err) {
      let message = "Failed to register feed.";
      if (err instanceof ApiError) {
        message = err.message;
      } else if (err instanceof Error) {
        message = err.message;
      }
      setActionMessage({ type: "error", text: message });
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleDeleteRequested = (link: FeedLink) => {
    setSelectedLink(link);
    onOpen();
  };

  const handleDeleteConfirmed = async () => {
    if (!selectedLink) return;

    setIsDeleting(true);
    try {
      await feedApi.deleteFeedLink(selectedLink.id);
      setActionMessage({ type: "success", text: "Feed link deleted." });
      await loadFeedLinks();
    } catch (err) {
      let message = "Failed to delete feed link.";
      if (err instanceof ApiError) {
        message = err.message;
      } else if (err instanceof Error) {
        message = err.message;
      }
      setActionMessage({ type: "error", text: message });
    } finally {
      setIsDeleting(false);
      onClose();
      setSelectedLink(null);
    }
  };

  return (
    <Box minH="100vh" bg="var(--app-bg)" px={4} py={10}>
      <Box maxW="container.lg" mx="auto">
        <Stack gap={10}>
          <Box>
            <Heading size="xl" color="var(--text-primary)" mb={2}>
              Manage Feeds Links
            </Heading>
            <Text color="var(--alt-text-secondary)">
              Add or remove the RSS sources that Alt will scan for your tenant.
            </Text>
          </Box>

          {actionMessage && (
            <Box
              bg={
                actionMessage.type === "success"
                  ? "var(--alt-success)"
                  : "var(--alt-error)"
              }
              color="white"
              borderRadius="lg"
              p={4}
            >
              <Flex align="flex-start" gap={3}>
                <Box flexShrink={0} mt={0.5}>
                  {actionMessage.type === "success" ? "✓" : "✕"}
                </Box>
                <Box flex={1}>
                  <Text fontWeight="bold" mb={1}>
                    {actionMessage.type === "success" ? "Success" : "Error"}
                  </Text>
                  <Text fontSize="sm">{actionMessage.text}</Text>
                </Box>
              </Flex>
            </Box>
          )}

          <Stack
            gap={6}
            direction={{ base: "column", lg: "row" }}
            align="flex-start"
          >
            <Box flex={1} w="full" className="glass" p={6} borderRadius="2xl">
              <Flex align="center" justify="space-between" mb={4}>
                <Heading size="md">Registered Feeds</Heading>
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={loadFeedLinks}
                  loading={isLoadingLinks}
                >
                  <Flex align="center" gap={2}>
                    <RefreshCw size={14} />
                    Refresh
                  </Flex>
                </Button>
              </Flex>

              {isLoadingLinks ? (
                <Flex align="center" justify="center" py={10}>
                  <Spinner color="var(--alt-primary)" />
                </Flex>
              ) : loadingError ? (
                <Box
                  bg="var(--alt-error)"
                  color="white"
                  borderRadius="md"
                  p={3}
                >
                  <Text fontSize="sm">{loadingError}</Text>
                </Box>
              ) : sortedLinks.length === 0 ? (
                <Text color="var(--text-muted)">No feeds registered yet.</Text>
              ) : (
                <Stack gap={3}>
                  {sortedLinks.map((link) => (
                    <Flex
                      key={link.id}
                      align="center"
                      justify="space-between"
                      px={4}
                      py={3}
                      bg="var(--surface-bg)"
                      border="1px solid var(--surface-border)"
                      borderRadius="xl"
                    >
                      <Text fontSize="sm" fontWeight="medium" truncate>
                        {link.url}
                      </Text>
                      <Box
                        as="button"
                        aria-label="Delete feed link"
                        onClick={() => handleDeleteRequested(link)}
                        display="flex"
                        alignItems="center"
                        justifyContent="center"
                        bg="transparent"
                        border="none"
                        cursor="pointer"
                        color="var(--alt-error)"
                        _hover={{ opacity: 0.7 }}
                        p={2}
                      >
                        <Trash2 size={16} />
                      </Box>
                    </Flex>
                  ))}
                </Stack>
              )}
            </Box>

            <Box flex={1} w="full" className="glass" p={6} borderRadius="2xl">
              <Heading size="md" mb={3}>
                Add New Feed
              </Heading>
              <Text mb={4} color="var(--text-muted)">
                Enter the RSS URL of a feed you want to track. Alt will validate
                the URL before scheduling the fetch.
              </Text>
              <form onSubmit={handleSubmit}>
                <Stack gap={4}>
                  <Input
                    type="url"
                    placeholder="https://example.com/feed.xml"
                    value={feedUrl}
                    onChange={handleUrlChange}
                    borderColor={
                      validationError
                        ? "var(--alt-error)"
                        : "var(--surface-border)"
                    }
                    bg="white"
                  />
                  {validationError && (
                    <Text color="var(--alt-error)" fontSize="sm">
                      {validationError}
                    </Text>
                  )}
                  <Button
                    type="submit"
                    bg="var(--alt-primary)"
                    color="white"
                    loading={isSubmitting}
                  >
                    Add Feed
                  </Button>
                </Stack>
              </form>
            </Box>
          </Stack>
        </Stack>
      </Box>

      <Dialog.Root
        open={open}
        onOpenChange={(e) => {
          if (!e.open) {
            onClose();
            setSelectedLink(null);
          }
        }}
      >
        <Portal>
          <Dialog.Backdrop bg="rgba(0, 0, 0, 0.6)" backdropFilter="blur(8px)" />
          <Dialog.Positioner>
            <Dialog.Content
              bg="var(--surface-bg)"
              border="1px solid var(--surface-border)"
              borderRadius="xl"
              maxW="500px"
            >
              <Dialog.Header fontSize="lg" fontWeight="bold" pb={3}>
                Delete feed link?
              </Dialog.Header>
              <Dialog.Body pb={4}>
                <Text>
                  Deleting <strong>{selectedLink?.url}</strong> will remove it
                  from the registry and stop Alt from checking it. This action
                  cannot be undone.
                </Text>
              </Dialog.Body>
              <Dialog.Footer gap={3}>
                <Button
                  ref={cancelRef}
                  onClick={() => {
                    onClose();
                    setSelectedLink(null);
                  }}
                  variant="outline"
                >
                  Cancel
                </Button>
                <Button
                  colorPalette="red"
                  onClick={handleDeleteConfirmed}
                  loading={isDeleting}
                >
                  Delete feed link
                </Button>
              </Dialog.Footer>
            </Dialog.Content>
          </Dialog.Positioner>
        </Portal>
      </Dialog.Root>
    </Box>
  );
}
