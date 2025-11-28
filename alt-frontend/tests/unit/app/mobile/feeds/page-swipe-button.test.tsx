import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import { render, screen, waitFor } from "@testing-library/react";
import type React from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { FeedsClient } from "@/app/mobile/feeds/_components/FeedsClient";

// Mock dependencies
vi.mock("@/lib/api", () => ({
  feedApi: {
    getFeedsWithCursor: vi.fn(),
    updateFeedReadStatus: vi.fn(),
  },
  feedsApi: {
    getFeedsWithCursor: vi.fn(),
    updateFeedReadStatus: vi.fn(),
  },
}));

vi.mock("@/hooks/useCursorPagination", () => ({
  useCursorPagination: () => ({
    data: [],
    hasMore: false,
    isLoading: false,
    error: null,
    isInitialLoading: false,
    loadMore: vi.fn(),
    refresh: vi.fn(),
  }),
}));

vi.mock("@/contexts/auth-context", () => ({
  useAuth: () => ({
    isAuthenticated: true,
    isLoading: false,
    user: { id: "test-user" },
  }),
}));

vi.mock("@/components/mobile/utils/FloatingMenu", () => ({
  FloatingMenu: () => <div data-testid="floating-menu">FloatingMenu</div>,
}));

vi.mock("@/components/mobile/VirtualFeedList", () => ({
  default: () => <div data-testid="virtual-feed-list">VirtualFeedList</div>,
}));

vi.mock("@/lib/utils/infiniteScroll", () => ({
  useInfiniteScroll: vi.fn(),
}));

vi.mock("@/lib/api/utils/serverFetch", () => ({
  serverFetch: vi.fn().mockResolvedValue({
    data: [],
  }),
}));

// Remove FeedsClient mock - test the actual component

vi.mock("next/navigation", () => ({
  useRouter: () => ({
    push: vi.fn(),
    replace: vi.fn(),
    prefetch: vi.fn(),
    back: vi.fn(),
    forward: vi.fn(),
    refresh: vi.fn(),
  }),
  usePathname: () => "/mobile/feeds",
  useSearchParams: () => new URLSearchParams(),
}));

const renderWithProviders = (ui: React.ReactElement) =>
  render(<ChakraProvider value={defaultSystem}>{ui}</ChakraProvider>);

describe("FeedsPage - Swipe Button", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should render swipe mode button with infinity icon", async () => {
    renderWithProviders(<FeedsClient initialFeeds={[]} />);

    // Wait for component to render
    await waitFor(
      () => {
        const button = screen.queryByTestId("swipe-mode-button");
        expect(button).toBeInTheDocument();
      },
      { timeout: 3000 },
    );

    const button = screen.getByTestId("swipe-mode-button");
    expect(button).toBeInTheDocument();
    expect(button).toHaveAttribute("aria-label", "Open swipe mode");
  });

  it("should have correct link to swipe mode", async () => {
    renderWithProviders(<FeedsClient initialFeeds={[]} />);

    // Wait for component to render
    // Use getAllByTestId since there might be multiple instances during hydration
    await waitFor(
      () => {
        const buttons = screen.queryAllByTestId("swipe-mode-button");
        expect(buttons.length).toBeGreaterThan(0);
      },
      { timeout: 3000 },
    );

    // Get the first button (or use getAllByTestId if multiple are expected)
    const buttons = screen.getAllByTestId("swipe-mode-button");
    const button = buttons[0];
    const link = button.closest("a");
    expect(link).toHaveAttribute("href", "/mobile/feeds/swipe");
  });
});
