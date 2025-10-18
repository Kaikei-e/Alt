import React from "react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import SwipeFeedsPage from "@/app/mobile/feeds/swipe/page";
import type { Feed } from "@/schema/feed";

const { mockUseSwipeFeedController } = vi.hoisted(() => ({
  mockUseSwipeFeedController: vi.fn(),
}));

vi.mock("@/components/mobile/feeds/swipe/useSwipeFeedController", () => ({
  useSwipeFeedController: () => mockUseSwipeFeedController(),
}));

vi.mock("@/components/mobile/feeds/swipe/SwipeFeedCard", () => ({
  __esModule: true,
  default: ({
    feed,
    statusMessage,
  }: {
    feed: Feed;
    statusMessage: string | null;
  }) => (
    <div data-testid="mock-swipe-card">
      <h3>{feed.title}</h3>
      {statusMessage ? <span>{statusMessage}</span> : null}
    </div>
  ),
}));

vi.mock("@/components/mobile/SkeletonFeedCard", () => ({
  __esModule: true,
  default: () => <div data-testid="mock-skeleton-card" />,
}));

vi.mock("@/components/mobile/EmptyFeedState", () => ({
  __esModule: true,
  default: () => <div data-testid="mock-empty-state">No feeds yet</div>,
}));

vi.mock("@/app/mobile/feeds/_components/ErrorState", () => ({
  __esModule: true,
  default: ({ error }: { error: Error }) => (
    <div data-testid="mock-error-state">{error.message}</div>
  ),
}));

vi.mock("@/components/mobile/utils/FloatingMenu", () => ({
  FloatingMenu: () => <div data-testid="floating-menu">FloatingMenu</div>,
}));

const renderWithProviders = (ui: React.ReactElement) =>
  render(<ChakraProvider value={defaultSystem}>{ui}</ChakraProvider>);

const baseState = () => ({
  feeds: [] as Feed[],
  activeFeed: null as Feed | null,
  activeIndex: 0,
  hasMore: false,
  isInitialLoading: false,
  isValidating: false,
  error: null as Error | null,
  liveRegionMessage: "",
  statusMessage: null as string | null,
  dismissActiveFeed: vi.fn(),
  retry: vi.fn(),
});

describe("SwipeFeedsPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders skeleton while initial data is loading", () => {
    mockUseSwipeFeedController.mockReturnValue({
      ...baseState(),
      isInitialLoading: true,
      hasMore: true,
    });

    renderWithProviders(<SwipeFeedsPage />);

    expect(screen.getByTestId("mock-skeleton-card")).toBeInTheDocument();
  });

  it("renders swipe card when an active feed is available", () => {
    const feed: Feed = {
      id: "feed-1",
      title: "Feed One",
      link: "#",
      published: "",
      description: "",
      feed_url: "#",
    };

    mockUseSwipeFeedController.mockReturnValue({
      ...baseState(),
      feeds: [feed],
      activeFeed: feed,
      statusMessage: "Ready",
      hasMore: true,
    });

    renderWithProviders(<SwipeFeedsPage />);

    expect(screen.getByTestId("mock-swipe-card")).toHaveTextContent("Feed One");
    expect(screen.getByText("Ready")).toBeInTheDocument();
    expect(screen.getByTestId("floating-menu")).toBeInTheDocument();
  });

  it("renders empty state when no feeds remain and there is no more data", () => {
    mockUseSwipeFeedController.mockReturnValue({
      ...baseState(),
      feeds: [],
      activeFeed: null,
      hasMore: false,
      isValidating: false,
    });

    renderWithProviders(<SwipeFeedsPage />);

    expect(screen.getByTestId("mock-empty-state")).toBeInTheDocument();
  });

  it("renders error state when hook reports an error", () => {
    mockUseSwipeFeedController.mockReturnValue({
      ...baseState(),
      error: new Error("Failed to load"),
      isValidating: false,
    });

    renderWithProviders(<SwipeFeedsPage />);

    expect(screen.getByTestId("mock-error-state")).toHaveTextContent(
      "Failed to load",
    );
  });
});
