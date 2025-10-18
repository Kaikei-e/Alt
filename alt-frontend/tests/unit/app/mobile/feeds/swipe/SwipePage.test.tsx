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

const { mockUseSwipeFeedController } = vi.hoisted(() => ({
  mockUseSwipeFeedController: vi.fn(),
}));

vi.mock("@/components/mobile/feeds/swipe/useSwipeFeedController", () => ({
  useSwipeFeedController: () => mockUseSwipeFeedController(),
}));

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

vi.mock("@/components/mobile/feeds/swipe/SwipeFeedCard", () => ({
  __esModule: true,
  default: ({
    feed,
    onDismiss,
    statusMessage,
  }: {
    feed: { id: string; title: string };
    statusMessage: string | null;
    onDismiss: (direction: number) => Promise<void> | void;
  }) => (
    <div>
      <h2>{feed.title}</h2>
      {statusMessage && (
        <p data-testid="swipe-status-message">{statusMessage}</p>
      )}
      <button
        type="button"
        onClick={() => onDismiss(1)}
      >
        Mark current feed as read
      </button>
    </div>
  ),
}));

import SwipePage from "@/app/mobile/feeds/swipe/page";

describe("/mobile/feeds/swipe page", () => {
  const renderPage = () =>
    render(
      <ChakraProvider value={defaultSystem}>
        <SwipePage />
      </ChakraProvider>,
    );

  beforeEach(() => {
    mockUseSwipeFeedController.mockReset();
    vi.useRealTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("renders the first fetched feed card", () => {
    const dismissActiveFeed = vi.fn();
    mockUseSwipeFeedController.mockReturnValue({
      feeds: [
        {
          id: "feed-1",
          title: "Sample feed",
        },
      ],
      activeFeed: { id: "feed-1", title: "Sample feed" },
      activeIndex: 0,
      hasMore: false,
      isInitialLoading: false,
      isValidating: false,
      error: null,
      liveRegionMessage: "",
      statusMessage: null,
      dismissActiveFeed,
      retry: vi.fn(),
    });

    renderPage();

    expect(screen.getByText("Sample feed")).toBeInTheDocument();
    expect(screen.getByTestId("floating-menu")).toBeInTheDocument();
  });

  it("marks a feed as read via the swipe controller", async () => {
    const feeds = [
      { id: "feed-1", title: "Swipe me" },
      { id: "feed-2", title: "Next feed" },
    ];

    const controllerState = {
      feeds,
      activeFeed: feeds[0],
      activeIndex: 0,
      hasMore: true,
      isInitialLoading: false,
      isValidating: false,
      error: null,
      liveRegionMessage: "",
      statusMessage: null as string | null,
      retry: vi.fn(),
      dismissActiveFeed: vi.fn(async () => {
        controllerState.feeds = feeds.slice(1);
        controllerState.activeFeed = feeds[1];
        controllerState.statusMessage = "Feed marked as read";
      }),
    };

    mockUseSwipeFeedController.mockImplementation(() => controllerState);

    const view = renderPage();

    const markButton = screen.getByRole("button", {
      name: /mark current feed as read/i,
    });

    await act(async () => {
      fireEvent.click(markButton);
    });

    expect(controllerState.dismissActiveFeed).toHaveBeenCalledWith(1);

    view.rerender(
      <ChakraProvider value={defaultSystem}>
        <SwipePage />
      </ChakraProvider>,
    );

    expect(screen.getByText("Next feed")).toBeInTheDocument();
    expect(screen.getByTestId("swipe-status-message")).toHaveTextContent(
      "Feed marked as read",
    );
  });
});
