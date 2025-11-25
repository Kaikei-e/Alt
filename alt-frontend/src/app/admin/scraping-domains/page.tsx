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
  Text,
  useDisclosure,
  VStack,
} from "@chakra-ui/react";
import { ArrowLeft, Home, RefreshCw, Settings } from "lucide-react";
import { useEffect, useState } from "react";
import Link from "next/link";
import {
  useScrapingDomains,
  useScrapingDomain,
  updateScrapingDomain,
  refreshRobotsTxt,
} from "@/lib/api/scrapingDomains";
import type { ScrapingDomain, UpdateScrapingDomainRequest } from "@/schema/scrapingDomain";
import { ApiError } from "@/lib/api/core/ApiError";
import { FloatingMenu } from "@/components/mobile/utils/FloatingMenu";

type ActionMessage = {
  type: "success" | "error";
  text: string;
};

export default function ScrapingDomainsPage() {
  const [offset, setOffset] = useState(0);
  const limit = 20;
  const { domains, isLoading, error, mutate } = useScrapingDomains(offset, limit);
  const [actionMessage, setActionMessage] = useState<ActionMessage | null>(null);
  const [selectedDomain, setSelectedDomain] = useState<ScrapingDomain | null>(null);
  const [isUpdating, setIsUpdating] = useState(false);
  const [isRefreshing, setIsRefreshing] = useState<string | null>(null);
  const { open, onOpen, onClose } = useDisclosure();

  const handleEdit = (domain: ScrapingDomain) => {
    setSelectedDomain(domain);
    onOpen();
  };

  const handleUpdate = async (data: UpdateScrapingDomainRequest) => {
    if (!selectedDomain) return;

    setIsUpdating(true);
    setActionMessage(null);

    try {
      await updateScrapingDomain(selectedDomain.id, data);
      setActionMessage({ type: "success", text: "Scraping domain policy updated." });
      await mutate();
      onClose();
    } catch (err) {
      let message = "Failed to update scraping domain policy.";
      if (err instanceof ApiError) {
        message = err.message;
      } else if (err instanceof Error) {
        message = err.message;
      }
      setActionMessage({ type: "error", text: message });
    } finally {
      setIsUpdating(false);
    }
  };

  const handleRefreshRobotsTxt = async (id: string) => {
    setIsRefreshing(id);
    setActionMessage(null);

    try {
      await refreshRobotsTxt(id);
      setActionMessage({ type: "success", text: "robots.txt refreshed successfully." });
      await mutate();
    } catch (err) {
      let message = "Failed to refresh robots.txt.";
      if (err instanceof ApiError) {
        message = err.message;
      } else if (err instanceof Error) {
        message = err.message;
      }
      setActionMessage({ type: "error", text: message });
    } finally {
      setIsRefreshing(null);
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
                  Scraping Domains
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
              Manage domain-level scraping policies and robots.txt settings.
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

          {/* Scraping Domains List */}
          <Box
            bg="var(--surface-bg)"
            border="1px solid var(--surface-border)"
            borderRadius="xl"
            p={5}
          >
            <Flex align="center" justify="space-between" mb={4}>
              <Heading size="sm">Scraping Domains</Heading>
              <IconButton
                aria-label="Refresh"
                size="sm"
                variant="ghost"
                onClick={() => mutate()}
                disabled={isLoading}
              >
                {isLoading ? <Spinner size="sm" /> : <RefreshCw size={16} />}
              </IconButton>
            </Flex>

            {isLoading ? (
              <Flex align="center" justify="center" py={10}>
                <Spinner color="var(--alt-primary)" size="lg" />
              </Flex>
            ) : error ? (
              <Box
                bg="var(--alt-error)"
                color="white"
                borderRadius="md"
                p={3}
                fontSize="sm"
              >
                <Text fontSize="xs">Failed to load scraping domains.</Text>
              </Box>
            ) : domains.length === 0 ? (
              <Text color="var(--text-muted)" fontSize="sm" textAlign="center" py={6}>
                No scraping domains found.
              </Text>
            ) : (
              <VStack gap={3} align="stretch">
                {domains.map((domain) => (
                  <Box
                    key={domain.id}
                    bg="var(--app-bg)"
                    border="1px solid var(--surface-border)"
                    borderRadius="lg"
                    p={4}
                  >
                    <VStack gap={3} align="stretch">
                      {/* Domain Header */}
                      <Flex align="center" justify="space-between">
                        <Text
                          fontSize="sm"
                          fontWeight="bold"
                          color="var(--text-primary)"
                          flex={1}
                        >
                          {domain.domain}
                        </Text>
                        <Flex gap={2}>
                          <IconButton
                            aria-label="Edit policy"
                            size="sm"
                            variant="ghost"
                            onClick={() => handleEdit(domain)}
                          >
                            <Settings size={16} />
                          </IconButton>
                          <IconButton
                            aria-label="Refresh robots.txt"
                            size="sm"
                            variant="ghost"
                            loading={isRefreshing === domain.id}
                            onClick={() => handleRefreshRobotsTxt(domain.id)}
                          >
                            <RefreshCw size={16} />
                          </IconButton>
                        </Flex>
                      </Flex>

                      {/* Policy Info */}
                      <VStack gap={2} align="stretch" fontSize="xs">
                        <Flex justify="space-between">
                          <Text color="var(--text-muted)">Allow Fetch Body:</Text>
                          <Text
                            color={
                              domain.allow_fetch_body
                                ? "var(--alt-success)"
                                : "var(--alt-error)"
                            }
                            fontWeight="medium"
                          >
                            {domain.allow_fetch_body ? "Yes" : "No"}
                          </Text>
                        </Flex>
                        <Flex justify="space-between">
                          <Text color="var(--text-muted)">Force Respect Robots:</Text>
                          <Text
                            color={
                              domain.force_respect_robots
                                ? "var(--alt-success)"
                                : "var(--text-muted)"
                            }
                            fontWeight="medium"
                          >
                            {domain.force_respect_robots ? "Yes" : "No"}
                          </Text>
                        </Flex>
                        <Flex justify="space-between">
                          <Text color="var(--text-muted)">Cache Days:</Text>
                          <Text color="var(--text-primary)" fontWeight="medium">
                            {domain.allow_cache_days}
                          </Text>
                        </Flex>
                        <Flex justify="space-between">
                          <Text color="var(--text-muted)">Robots.txt Status:</Text>
                          <Text
                            color={
                              domain.robots_txt_last_status === 200
                                ? "var(--alt-success)"
                                : domain.robots_txt_last_status
                                  ? "var(--alt-error)"
                                  : "var(--text-muted)"
                            }
                            fontWeight="medium"
                          >
                            {domain.robots_txt_last_status
                              ? `HTTP ${domain.robots_txt_last_status}`
                              : "Not fetched"}
                          </Text>
                        </Flex>
                        {domain.robots_disallow_paths.length > 0 && (
                          <Box>
                            <Text color="var(--text-muted)" mb={1}>
                              Disallowed Paths:
                            </Text>
                            <VStack gap={1} align="stretch">
                              {domain.robots_disallow_paths.map((path, idx) => (
                                <Text
                                  key={idx}
                                  color="var(--alt-error)"
                                  fontSize="xs"
                                  pl={2}
                                >
                                  • {path}
                                </Text>
                              ))}
                            </VStack>
                          </Box>
                        )}
                      </VStack>
                    </VStack>
                  </Box>
                ))}
              </VStack>
            )}
          </Box>
        </VStack>
      </Box>

      {/* Floating Menu */}
      <FloatingMenu />

      {/* Edit Domain Modal */}
      <EditDomainModal
        isOpen={open}
        onClose={onClose}
        domain={selectedDomain}
        onUpdate={handleUpdate}
        isUpdating={isUpdating}
      />
    </Box>
  );
}

function EditDomainModal({
  isOpen,
  onClose,
  domain,
  onUpdate,
  isUpdating,
}: {
  isOpen: boolean;
  onClose: () => void;
  domain: ScrapingDomain | null;
  onUpdate: (data: UpdateScrapingDomainRequest) => Promise<void>;
  isUpdating: boolean;
}) {
  const [allowFetchBody, setAllowFetchBody] = useState(false);
  const [allowMLTraining, setAllowMLTraining] = useState(false);
  const [allowCacheDays, setAllowCacheDays] = useState(7);
  const [forceRespectRobots, setForceRespectRobots] = useState(false);

  // Update form when domain or modal state changes
  useEffect(() => {
    if (domain && isOpen) {
      setAllowFetchBody(domain.allow_fetch_body);
      setAllowMLTraining(domain.allow_ml_training);
      setAllowCacheDays(domain.allow_cache_days);
      setForceRespectRobots(domain.force_respect_robots);
    }
  }, [domain, isOpen]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    await onUpdate({
      allow_fetch_body: allowFetchBody,
      allow_ml_training: allowMLTraining,
      allow_cache_days: allowCacheDays,
      force_respect_robots: forceRespectRobots,
    });
  };

  return (
    <Dialog.Root
      open={isOpen}
      onOpenChange={(e) => {
        if (!e.open) {
          onClose();
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
              Edit Scraping Domain Policy
            </Dialog.Header>
            <Dialog.Body pb={4}>
              {domain && (
                <VStack gap={4} align="stretch">
                  <Box>
                    <Text fontSize="sm" color="var(--text-muted)" mb={2}>
                      Domain:
                    </Text>
                    <Text fontSize="sm" fontWeight="bold" color="var(--text-primary)">
                      {domain.domain}
                    </Text>
                  </Box>

                  <Flex align="center" gap={2} as="label">
                    <input
                      type="checkbox"
                      checked={allowFetchBody}
                      onChange={(e) => setAllowFetchBody(e.target.checked)}
                      style={{ accentColor: "var(--alt-primary)" }}
                    />
                    <Text fontSize="sm">Allow Fetch Body</Text>
                  </Flex>

                  <Flex align="center" gap={2} as="label">
                    <input
                      type="checkbox"
                      checked={allowMLTraining}
                      onChange={(e) => setAllowMLTraining(e.target.checked)}
                      style={{ accentColor: "var(--alt-primary)" }}
                    />
                    <Text fontSize="sm">Allow ML Training</Text>
                  </Flex>

                  <Box>
                    <Text fontSize="sm" color="var(--text-muted)" mb={2}>
                      Cache Days
                    </Text>
                    <Input
                      type="number"
                      value={allowCacheDays}
                      onChange={(e) => {
                        const value = parseInt(e.target.value, 10);
                        if (!isNaN(value) && value >= 0 && value <= 365) {
                          setAllowCacheDays(value);
                        }
                      }}
                      min={0}
                      max={365}
                      size="md"
                    />
                  </Box>

                  <Flex align="center" gap={2} as="label">
                    <input
                      type="checkbox"
                      checked={forceRespectRobots}
                      onChange={(e) => setForceRespectRobots(e.target.checked)}
                      style={{ accentColor: "var(--alt-primary)" }}
                    />
                    <Text fontSize="sm">Force Respect Robots.txt</Text>
                  </Flex>
                </VStack>
              )}
            </Dialog.Body>
            <Dialog.Footer gap={3}>
              <Button
                onClick={() => {
                  onClose();
                }}
                size="md"
                variant="outline"
              >
                Cancel
              </Button>
              <Button onClick={handleSubmit} loading={isUpdating} size="md">
                Save
              </Button>
            </Dialog.Footer>
          </Dialog.Content>
        </Dialog.Positioner>
      </Portal>
    </Dialog.Root>
  );
}
