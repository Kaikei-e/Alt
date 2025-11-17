"use client";

import {
  Box,
  Button,
  Dialog,
  Flex,
  Heading,
  IconButton,
  Input,
  Portal,
  Spinner,
  Stack,
  Text,
  useDisclosure,
  VStack,
} from "@chakra-ui/react";
import { ArrowLeft, Home, RefreshCw, Trash2, Plus } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import * as v from "valibot";
import { ApiError } from "@/lib/api/core/ApiError";
import { feedApi } from "@/lib/api";
import { feedUrlSchema } from "@/schema/validation/feedUrlSchema";
import type { FeedLink } from "@/schema/feedLink";
import { FloatingMenu } from "@/components/mobile/utils/FloatingMenu";
import Link from "next/link";

type ActionMessage = {
  type: "success" | "error";
  text: string;
};

const validateUrl = (url: string): string | null => {
  if (!url.trim()) return "Please enter the RSS URL.";

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
  const [actionMessage, setActionMessage] = useState<ActionMessage | null>(null);
  const [showAddForm, setShowAddForm] = useState(false);
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
    setShowAddForm(false);
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
      setActionMessage({ type: "success", text: "Feed registered successfully." });
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
    <Box
      minH="100dvh"
      bg="var(--app-bg)"
      position="relative"
      overflowX="hidden"
      pt="env(safe-area-inset-top)"
      pb="calc(env(safe-area-inset-bottom) + 80px)"
    >
      <Box maxW="container.sm" mx="auto" px={4} py={6}>
        <VStack gap={6} align="stretch">
          {/* Header */}
          <Box>
            <Flex align="center" justify="space-between" mb={3}>
              <Flex align="center" gap={3}>
                <Link href="/mobile/feeds">
                  <Button
                    size="sm"
                    variant="ghost"
                    aria-label="Back to feeds list"
                    p={2}
                    minW="40px"
                    minH="40px"
                  >
                    <ArrowLeft size={18} />
                  </Button>
                </Link>
                <Heading size="lg" color="var(--text-primary)">
                  Feed Management
                </Heading>
              </Flex>
              <Link href="/">
                <Button
                  size="sm"
                  variant="ghost"
                  aria-label="Back to home"
                  p={2}
                  minW="40px"
                  minH="40px"
                >
                  <Home size={18} />
                </Button>
              </Link>
            </Flex>
            <Text color="var(--alt-text-secondary)" fontSize="sm" ml="52px">
              Add or remove the RSS sources that Alt will scan for your tenant.
            </Text>
          </Box>

          {/* Action Message */}
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
              fontSize="sm"
            >
              <Flex align="flex-start" gap={3}>
                <Box flexShrink={0} mt={0.5}>
                  {actionMessage.type === "success" ? "✓" : "✕"}
                </Box>
                <Box flex={1}>
                  <Text fontWeight="bold" fontSize="sm" mb={1}>
                    {actionMessage.type === "success" ? "Success" : "Error"}
                  </Text>
                  <Text fontSize="xs">{actionMessage.text}</Text>
                </Box>
              </Flex>
            </Box>
          )}

          {/* Add Feed Button (Mobile) */}
          {!showAddForm && (
            <Button
              bg="var(--alt-primary)"
              color="white"
              size="lg"
              onClick={() => setShowAddForm(true)}
            >
              <Flex align="center" gap={2}>
                <Plus size={18} />
                Add a new feed
              </Flex>
            </Button>
          )}

          {/* Add Feed Form (Mobile) */}
          {showAddForm && (
            <Box
              bg="var(--surface-bg)"
              border="1px solid var(--surface-border)"
              borderRadius="xl"
              p={5}
            >
              <Flex align="center" justify="space-between" mb={4}>
                <Heading size="sm">Add a new feed</Heading>
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => {
                    resetForm();
                    setActionMessage(null);
                  }}
                >
                  Cancel
                </Button>
              </Flex>
              <Text mb={4} color="var(--text-muted)" fontSize="sm">
                Please enter the RSS URL. Alt will validate the URL before scheduling the fetch.
              </Text>
              <form onSubmit={handleSubmit}>
                <VStack gap={4}>
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
                    size="lg"
                    fontSize="sm"
                  />
                  {validationError && (
                    <Text color="var(--alt-error)" fontSize="xs" w="full">
                      {validationError}
                    </Text>
                  )}
                  <Button
                    type="submit"
                    bg="var(--alt-primary)"
                    color="white"
                    loading={isSubmitting}
                    size="lg"
                    w="full"
                  >
                    Add feed
                  </Button>
                </VStack>
              </form>
            </Box>
          )}

          {/* Feed Links List */}
          <Box
            bg="var(--surface-bg)"
            border="1px solid var(--surface-border)"
            borderRadius="xl"
            p={5}
          >
            <Flex align="center" justify="space-between" mb={4}>
              <Heading size="sm">Registered feeds</Heading>
              <IconButton
                aria-label="Refresh"
                size="sm"
                variant="ghost"
                onClick={loadFeedLinks}
                disabled={isLoadingLinks}
              >
                {isLoadingLinks ? <Spinner size="sm" /> : <RefreshCw size={16} />}
              </IconButton>
            </Flex>

            {isLoadingLinks ? (
              <Flex align="center" justify="center" py={10}>
                <Spinner color="var(--alt-primary)" size="lg" />
              </Flex>
            ) : loadingError ? (
              <Box
                bg="var(--alt-error)"
                color="white"
                borderRadius="md"
                p={3}
                fontSize="sm"
              >
                <Text fontSize="xs">{loadingError}</Text>
              </Box>
            ) : sortedLinks.length === 0 ? (
              <Text color="var(--text-muted)" fontSize="sm" textAlign="center" py={6}>
                No feeds registered yet.
              </Text>
            ) : (
              <VStack gap={3} align="stretch">
                {sortedLinks.map((link) => (
                  <Flex
                    key={link.id}
                    align="center"
                    justify="space-between"
                    px={4}
                    py={3}
                    bg="var(--app-bg)"
                    border="1px solid var(--surface-border)"
                    borderRadius="lg"
                    minH="56px"
                  >
                    <Text
                      fontSize="sm"
                      fontWeight="medium"
                      truncate
                      flex={1}
                      mr={3}
                    >
                      {link.url}
                    </Text>
                    <Box
                      as="button"
                      aria-label="Delete feed link"
                      onClick={() => handleDeleteRequested(link)}
                      minW="44px"
                      minH="44px"
                      display="flex"
                      alignItems="center"
                      justifyContent="center"
                      bg="transparent"
                      border="none"
                      cursor="pointer"
                      color="var(--alt-error)"
                      _hover={{ opacity: 0.7 }}
                    >
                      <Trash2 size={18} />
                    </Box>
                  </Flex>
                ))}
              </VStack>
            )}
          </Box>
        </VStack>
      </Box>

      {/* Floating Menu */}
      <FloatingMenu />

      {/* Delete Confirmation Dialog */}
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
              mx={4}
              maxW="400px"
            >
              <Dialog.Header fontSize="lg" fontWeight="bold" pb={3}>
                Delete feed link?
              </Dialog.Header>
              <Dialog.Body pb={4}>
                <Text fontSize="sm">
                  <strong>{selectedLink?.url}</strong>
                  Deleting this feed link will remove it from the registry and stop Alt from checking it. This action cannot be undone.
                </Text>
              </Dialog.Body>
              <Dialog.Footer gap={3}>
                <Button
                  ref={cancelRef}
                  onClick={() => {
                    onClose();
                    setSelectedLink(null);
                  }}
                  size="md"
                  variant="outline"
                >
                  Cancel
                </Button>
                <Button
                  colorPalette="red"
                  onClick={handleDeleteConfirmed}
                  loading={isDeleting}
                  size="md"
                >
                  Delete
                </Button>
              </Dialog.Footer>
            </Dialog.Content>
          </Dialog.Positioner>
        </Portal>
      </Dialog.Root>
    </Box>
  );
}

