import React from "react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor, act } from "@testing-library/react";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";

vi.mock("@/contexts/auth-context", () => ({
  useAuth: () => ({
    isAuthenticated: true,
    isLoading: false,
    user: { id: "user-1" },
  }),
}));

const { mockUseSWRInfinite, mockUpdateFeedReadStatus } = vi.hoisted(() => ({
  mockUseSWRInfinite: vi.fn(),
  mockUpdateFeedReadStatus: vi.fn(),
}));

vi.mock("swr/infinite", () => ({
  default: (...args: unknown[]) => mockUseSWRInfinite(...args),
  useSWRInfinite: (...args: unknown[]) => mockUseSWRInfinite(...args),
}));

vi.mock("@/lib/api", async (importOriginal) => {
  const actual = await importOriginal();
  return {
    ...actual,
    feedsApi: {
      ...actual.feedsApi,
      getFeedsWithCursor: vi.fn(),
      updateFeedReadStatus: mockUpdateFeedReadStatus,
    },
  };
});

vi.mock("@/components/mobile/utils/FloatingMenu", () => ({
  FloatingMenu: () => <div data-testid="floating-menu" />,
}));

vi.mock("@use-gesture/react", () => ({
  useDrag: () => () => ({}),
}));

vi.mock("framer-motion", () => {
  const React = require("react");

  const stripMotionProps = (props: Record<string, unknown>) => {
    const {
      drag,
      dragElastic,
      initial,
      animate: motionAnimate,
      exit,
      variants,
      transition,
      layout,
      layoutId,
      whileTap,
      whileHover,
      whileDrag,
      ...rest
    } = props;
    return rest;
  };

  const motionFactory = (Component: React.ComponentType) =>
    React.forwardRef((props: Record<string, unknown>, ref) => (
      <Component ref={ref as never} {...stripMotionProps(props)} />
    ));

  motionFactory.div = React.forwardRef(
    (props: Record<string, unknown>, ref) => (
      <div ref={ref as never} {...stripMotionProps(props)} />
    ),
  );

  motionFactory.section = React.forwardRef(
    (props: Record<string, unknown>, ref) => (
      <section ref={ref as never} {...stripMotionProps(props)} />
    ),
  );

  return {
    AnimatePresence: ({ children }: { children: React.ReactNode }) => <>{children}</>,
    motion: motionFactory,
    useMotionValue: (initial = 0) => {
      let current = initial;
      return {
        set: (value: number) => {
          current = value;
        },
        get: () => current,
        jump: (value: number) => {
          current = value;
        },
      };
    },
    animate: vi.fn(),
  };
});

import SwipePage from "@/app/mobile/feeds/swipe/page";

describe("/mobile/feeds/swipe page", () => {
  const renderPage = () =>
    render(
      <ChakraProvider value={defaultSystem}>
        <SwipePage />
      </ChakraProvider>,
    );

  beforeEach(() => {
    mockUseSWRInfinite.mockReset();
    mockUpdateFeedReadStatus.mockReset();
    vi.useRealTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("renders the first fetched feed card", () => {
    mockUseSWRInfinite.mockReturnValue({
      data: [
        {
          data: [
            {
              id: "feed-1",
              title: "Sample feed",
              description: "Description", 
              link: "https://example.com/feed-1",
              published: "2025-01-01T00:00:00Z",
            },
          ],
          next_cursor: null,
        },
      ],
      error: undefined,
      size: 1,
      setSize: vi.fn(),
      mutate: vi.fn(),
      isLoading: false,
      isValidating: false,
    });

    renderPage();

    expect(screen.getByText("Sample feed")).toBeInTheDocument();
    expect(screen.getByTestId("floating-menu")).toBeInTheDocument();
  });

  it("marks a feed as read and prefetches when close to the threshold", async () => {
    const setSize = vi.fn();
    mockUseSWRInfinite.mockReturnValue({
      data: [
        {
          data: [
            {
              id: "feed-1",
              title: "Swipe me",
              description: "Desc",
              link: "https://example.com/article/?utm_source=test",
              published: "2025-01-01T00:00:00Z",
            },
            {
              id: "feed-2",
              title: "Next feed",
              description: "Desc",
              link: "https://example.com/feed-2",
              published: "2025-01-02T00:00:00Z",
            },
            {
              id: "feed-3",
              title: "Third feed",
              description: "Desc",
              link: "https://example.com/feed-3",
              published: "2025-01-03T00:00:00Z",
            },
            {
              id: "feed-4",
              title: "Fourth feed",
              description: "Desc",
              link: "https://example.com/feed-4",
              published: "2025-01-04T00:00:00Z",
            },
          ],
          next_cursor: "cursor-2",
        },
      ],
      error: undefined,
      size: 1,
      setSize,
      mutate: vi.fn(),
      isLoading: false,
      isValidating: false,
    });

    vi.useFakeTimers();

    renderPage();

    const markButton = screen.getByRole("button", {
      name: /mark current feed as read/i,
    });

    fireEvent.click(markButton);

    await act(async () => {
      vi.runAllTimers();
    });
    vi.useRealTimers();

    await waitFor(() =>
      expect(mockUpdateFeedReadStatus).toHaveBeenCalledWith(
        "https://example.com/article",
      ),
    );

    await waitFor(() => {
      expect(setSize).toHaveBeenCalledWith(2);
    });

    expect(screen.queryByText("Swipe me")).not.toBeInTheDocument();
    expect(screen.getByText("Next feed")).toBeInTheDocument();
  });
});
