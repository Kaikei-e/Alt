import React from "react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import FeedsPage from "@/app/mobile/feeds/page";

// Mock dependencies
vi.mock("@/lib/api", () => ({
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
    renderWithProviders(<FeedsPage />);

    await waitFor(() => {
      const button = screen.getByTestId("swipe-mode-button");
      expect(button).toBeInTheDocument();
      expect(button).toHaveAttribute("aria-label", "Open swipe mode");
    });
  });

  it("should have correct link to swipe mode", async () => {
    renderWithProviders(<FeedsPage />);

    await waitFor(() => {
      const link = screen.getByTestId("swipe-mode-button").closest("a");
      expect(link).toHaveAttribute("href", "/mobile/feeds/swipe");
    });
  });
});
