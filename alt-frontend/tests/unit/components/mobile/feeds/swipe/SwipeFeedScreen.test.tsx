import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import { cleanup, render, screen } from "@testing-library/react";
import type { ReactElement } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
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
    </ChakraProvider> as ReactElement,
  );

const mockMatchMedia = (reduceMotion: boolean) => {
  window.matchMedia = vi.fn().mockImplementation((query: string) => ({
    matches:
      query === "(prefers-reduced-motion: reduce)" ? reduceMotion : false,
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

  afterEach(() => {
    cleanup();
  });

  it("renders skeleton while data is unknown but validating", () => {
    mockedUseSwipeFeedController.mockReturnValue({
      ...baseState,
      feeds: [],
      activeFeed: undefined as unknown as Feed,
      isInitialLoading: false,
      isValidating: true,
      hasMore: true,
    });

    renderWithProviders();

    expect(screen.getByTestId("swipe-skeleton-container")).toBeInTheDocument();
    expect(screen.getByTestId("swipe-skeleton-card")).toBeInTheDocument();
  });

  it("shows empty state when no feeds remain even if validating", () => {
    mockedUseSwipeFeedController.mockReturnValue({
      ...baseState,
      feeds: [],
      activeFeed: undefined as unknown as Feed,
      hasMore: false,
      isInitialLoading: false,
      isValidating: true,
    });

    renderWithProviders();

    expect(screen.getByTestId("empty-state-icon")).toBeInTheDocument();
  });

  it("renders enhanced skeleton and hint during initial loading", () => {
    mockedUseSwipeFeedController.mockReturnValue({
      ...baseState,
      feeds: [],
      activeFeed: undefined as unknown as Feed,
      isInitialLoading: true,
    });

    renderWithProviders();

    expect(screen.getByTestId("swipe-skeleton-container")).toBeInTheDocument();
    expect(screen.getByTestId("swipe-skeleton-card")).toBeInTheDocument();
    expect(screen.getByTestId("swipe-skeleton-hint")).toBeInTheDocument();
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

    const hints = screen.getAllByTestId("swipe-skeleton-hint");
    const reducedMotionHint = hints.find(
      (hint) => hint.getAttribute("data-reduced-motion") === "true",
    );
    expect(reducedMotionHint).toBeInTheDocument();
    expect(reducedMotionHint).toHaveAttribute("data-reduced-motion", "true");
  });

  it("renders the empty state when feeds are exhausted without pending fetches", () => {
    mockedUseSwipeFeedController.mockReturnValue({
      ...baseState,
      feeds: [],
      activeFeed: undefined as unknown as Feed,
      hasMore: false,
      isInitialLoading: false,
      isValidating: false,
    });

    renderWithProviders();

    expect(screen.getByTestId("empty-state-icon")).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: /no feeds yet/i }),
    ).toBeInTheDocument();
  });

  it("continues to show skeleton when feeds array is empty but more pages exist", () => {
    mockedUseSwipeFeedController.mockReturnValue({
      ...baseState,
      feeds: [],
      activeFeed: undefined as unknown as Feed,
      hasMore: true,
      isInitialLoading: false,
      isValidating: true,
    });

    renderWithProviders();

    expect(screen.getByTestId("swipe-skeleton-container")).toBeInTheDocument();
    expect(screen.queryByTestId("empty-state-icon")).not.toBeInTheDocument();
  });
});
