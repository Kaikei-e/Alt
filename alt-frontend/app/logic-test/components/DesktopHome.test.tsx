import React from "react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { feedsApi } from "@/lib/api";

vi.mock("@/lib/api", () => ({
  feedsApi: {
    getFeedStats: vi.fn().mockResolvedValue({
      feed_amount: { amount: 1 },
      summarized_feed: { amount: 1 },
    }),
    getTodayUnreadCount: vi.fn().mockResolvedValue({ count: 3 }),
  },
}));

vi.mock("@/hooks/useTodayUnreadCount", async () => {
  const actual = await vi.importActual("@/hooks/useTodayUnreadCount");
  return {
    ...actual,
    useTodayUnreadCount: () => ({ count: 3, isLoading: false }),
  };
});

vi.mock("@/components/ThemeToggle", () => ({
  ThemeToggle: () => <div>toggle</div>,
}));

vi.mock("@/components/mobile/utils/Loading", () => ({
  default: () => <div>loading</div>,
}));

vi.mock("@/components/mobile/stats/AnimatedNumber", () => ({
  AnimatedNumber: ({ value }: { value: number }) => <span>{value}</span>,
}));

vi.mock("@chakra-ui/react", () => ({
  Box: ({ children }: any) => <div>{children}</div>,
  Flex: ({ children }: any) => <div>{children}</div>,
  Text: ({ children }: any) => <span>{children}</span>,
  VStack: ({ children }: any) => <div>{children}</div>,
  HStack: ({ children }: any) => <div>{children}</div>,
  Grid: ({ children }: any) => <div>{children}</div>,
  GridItem: ({ children }: any) => <div>{children}</div>,
  Button: ({ children }: any) => <button>{children}</button>,
  Icon: () => <span />,
  Link: ({ children }: any) => <a>{children}</a>,
  useBreakpointValue: () => "md",
}));

vi.mock("lucide-react", () => ({
  Home: () => <svg />,
  Rss: () => <svg />,
  FileText: () => <svg />,
  Search: () => <svg />,
  Settings: () => <svg />,
  Plus: () => <svg />,
  TrendingUp: () => <svg />,
  Clock: () => <svg />,
  ArrowRight: () => <svg />,
  Activity: () => <svg />,
  Bookmark: () => <svg />,
  Download: () => <svg />,
  Sun: () => <svg />,
  Moon: () => <svg />,
  Zap: () => <svg />,
}));

describe("DesktopHome component", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders unread count", async () => {
    const { default: DesktopHome } = await import(
      "@/components/desktop/home/Home"
    );
    render(<DesktopHome />);
    await waitFor(() => {
      expect(screen.getByText("Unread Articles")).toBeDefined();
    });
    expect(screen.getByText("3")).toBeDefined();
  });
});
