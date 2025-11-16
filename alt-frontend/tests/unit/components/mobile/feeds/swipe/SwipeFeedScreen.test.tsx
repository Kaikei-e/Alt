import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import SwipeFeedScreen from "@/components/mobile/feeds/swipe/SwipeFeedScreen";
import { useSwipeFeedController } from "@/components/mobile/feeds/swipe/useSwipeFeedController";
import type { Feed } from "@/schema/feed";

vi.mock("@/components/mobile/feeds/swipe/useSwipeFeedController", () => ({
  useSwipeFeedController: vi.fn(),
}));

vi.mock("@/components/mobile/utils/FloatingMenu", () => ({
  FloatingMenu: () => <div data-testid="floating-menu">menu</div>,
}));

const renderWithProviders = () =>
  render(
    <ChakraProvider value={defaultSystem}>
      <SwipeFeedScreen />
    </ChakraProvider>
  );

const mockMatchMedia = (reduceMotion: boolean) => {
  window.matchMedia = vi.fn().mockImplementation((query: string) => ({
    matches: query === "(prefers-reduced-motion: reduce)" ? reduceMotion : false,
    media: query,
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  }));
};

const fallbackFeed: Feed = {
  id: "placeholder",
  title: "Placeholder",
  description: "",
  link: "https://example.com/placeholder",
  published: "",
};

const baseState = {
  feeds: [] as Feed[],
  activeFeed: fallbackFeed,
  activeIndex: 0,
  hasMore: false,
  isInitialLoading: false,
  isValidating: false,
  error: null as Error | null,
  liveRegionMessage: "",
  statusMessage: null as string | null,
  dismissActiveFeed: vi.fn(),
  retry: vi.fn(),
  getCachedContent: vi.fn(),
};

const mockedUseSwipeFeedController = vi.mocked(useSwipeFeedController);

describe("SwipeFeedScreen", () => {
  beforeEach(() => {
    mockMatchMedia(false);
    vi.clearAllMocks();
  });

  it("renders enhanced skeleton and hint during initial loading", () => {
    mockedUseSwipeFeedController.mockReturnValue({
      ...baseState,
      isInitialLoading: true,
    });

    renderWithProviders();

    expect(screen.getByTestId("swipe-skeleton-container")).toBeInTheDocument();
    expect(screen.getByTestId("swipe-skeleton-card")).toBeInTheDocument();
    expect(screen.getByTestId("swipe-skeleton-hint")).toHaveAttribute("data-reduced-motion", "false");
  });

  it("shows progress overlay when validating additional feeds", () => {
    const feed: Feed = {
      id: "feed-1",
      title: "Example feed",
      description: "desc",
      link: "https://example.com",
      published: new Date().toISOString(),
    };

    mockedUseSwipeFeedController.mockReturnValue({
      ...baseState,
      feeds: [feed],
      activeFeed: feed,
      isValidating: true,
    });

    renderWithProviders();

    expect(screen.getByTestId("swipe-progress-indicator")).toBeInTheDocument();
    const announcements = screen.getAllByText("新しい記事を読み込んでいます");
    expect(announcements[0]).toBeVisible();
  });

  it("respects prefers-reduced-motion setting for hint animation", () => {
    mockMatchMedia(true);

    mockedUseSwipeFeedController.mockReturnValue({
      ...baseState,
      isInitialLoading: true,
    });

    renderWithProviders();

    expect(screen.getByTestId("swipe-skeleton-hint")).toHaveAttribute("data-reduced-motion", "true");
  });
});

