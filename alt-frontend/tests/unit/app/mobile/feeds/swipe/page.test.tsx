import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import SwipeFeedsPage from "@/app/mobile/feeds/swipe/page";
import { Feed } from "@/schema/feed";

// Mock dependencies
vi.mock("@/lib/api", () => ({
  feedsApi: {
    getFeedsWithCursor: vi.fn(),
    updateFeedReadStatus: vi.fn(),
  },
}));

vi.mock("@/components/mobile/utils/FloatingMenu", () => ({
  FloatingMenu: () => <div data-testid="floating-menu">FloatingMenu</div>,
}));

vi.mock("framer-motion", () => ({
  motion: {
    div: ({ children, ...props }: any) => <div {...props}>{children}</div>,
  },
  AnimatePresence: ({ children }: any) => <>{children}</>,
  useMotionValue: () => ({ set: vi.fn(), get: () => 0 }),
  animate: vi.fn(),
}));

vi.mock("@use-gesture/react", () => ({
  useDrag: () => () => ({}),
}));

const mockFeeds: Feed[] = Array.from({ length: 60 }, (_, i) => ({
  id: `feed-${i}`,
  title: `Feed ${i}`,
  link: `https://example.com/feed-${i}`,
  description: `Description for feed ${i}`,
  published: new Date().toISOString(),
  feed_url: `https://example.com/feed-${i}`,
}));

describe("SwipeFeedsPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should load multiple pages initially", async () => {
    const { feedsApi } = await import("@/lib/api");

    // Mock returns 20 feeds per page with cursor
    vi.mocked(feedsApi.getFeedsWithCursor).mockImplementation(
      async (cursor?: string) => {
        const start = cursor ? parseInt(cursor) : 0;
        return {
          data: mockFeeds.slice(start, start + 20),
          next_cursor: start + 20 < mockFeeds.length ? `${start + 20}` : null,
        };
      },
    );

    render(<SwipeFeedsPage />);

    // Wait for initial load
    await waitFor(() => {
      expect(feedsApi.getFeedsWithCursor).toHaveBeenCalled();
    });

    // Should call API multiple times for initial pages (initialSize = 3)
    await waitFor(
      () => {
        expect(feedsApi.getFeedsWithCursor).toHaveBeenCalledTimes(3);
      },
      { timeout: 3000 },
    );
  });

  it("should display swipe card when feeds are loaded", async () => {
    const { feedsApi } = await import("@/lib/api");

    vi.mocked(feedsApi.getFeedsWithCursor).mockResolvedValue({
      data: mockFeeds.slice(0, 20),
      next_cursor: "20",
    });

    render(<SwipeFeedsPage />);

    await waitFor(() => {
      expect(screen.getByTestId("swipe-card")).toBeInTheDocument();
    });
  });

  it("should show FloatingMenu", async () => {
    const { feedsApi } = await import("@/lib/api");

    vi.mocked(feedsApi.getFeedsWithCursor).mockResolvedValue({
      data: mockFeeds.slice(0, 20),
      next_cursor: "20",
    });

    render(<SwipeFeedsPage />);

    await waitFor(() => {
      expect(screen.getByTestId("floating-menu")).toBeInTheDocument();
    });
  });
});
