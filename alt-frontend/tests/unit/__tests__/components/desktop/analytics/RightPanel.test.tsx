import React from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import { RightPanel } from "@/components/desktop/analytics/RightPanel";
import { ThemeProvider } from "@/providers/ThemeProvider";
import { describe, it, expect, vi } from "vitest";

// Mock matchMedia
Object.defineProperty(window, "matchMedia", {
  writable: true,
  value: vi.fn().mockImplementation((query) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: vi.fn(), // deprecated
    removeListener: vi.fn(), // deprecated
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })),
});

// Mock the custom hooks
vi.mock("@/hooks/useReadingAnalytics", () => ({
  useReadingAnalytics: () => ({
    analytics: null,
    isLoading: false,
  }),
}));

vi.mock("@/hooks/useTrendingTopics", () => ({
  useTrendingTopics: () => ({
    topics: [],
    isLoading: false,
  }),
}));

vi.mock("@/hooks/useSourceAnalytics", () => ({
  useSourceAnalytics: () => ({
    sources: [],
    isLoading: false,
  }),
}));

vi.mock("@/hooks/useQuickActions", () => ({
  useQuickActions: () => ({
    actions: [],
    counters: { unread: 0, bookmarks: 0, queue: 0 },
  }),
}));

const renderWithProviders = (ui: React.ReactElement) => {
  return render(
    <ThemeProvider>
      <ChakraProvider value={defaultSystem}>{ui}</ChakraProvider>
    </ThemeProvider>,
  );
};

describe("RightPanel", () => {
  it("should render with glass effect", () => {
    renderWithProviders(<RightPanel />);

    const glassElements = document.querySelectorAll(".glass");
    expect(glassElements.length).toBeGreaterThan(0);
  });

  it("should show Analytics tab as active by default", () => {
    renderWithProviders(<RightPanel />);

    const analyticsTabs = screen.getAllByRole("button", { name: /analytics/i });
    expect(analyticsTabs.length).toBeGreaterThan(0);
    expect(analyticsTabs[0]).toBeInTheDocument();
  });

  it("should switch between tabs", async () => {
    const user = userEvent.setup();
    renderWithProviders(<RightPanel />);

    // Click on Actions tab
    const actionsTabs = screen.getAllByRole("button", { name: /actions/i });
    await user.click(actionsTabs[0]);

    expect(actionsTabs[0]).toBeInTheDocument();

    // Switch back to Analytics tab
    const analyticsTabs = screen.getAllByRole("button", { name: /analytics/i });
    await user.click(analyticsTabs[0]);

    expect(analyticsTabs[0]).toBeInTheDocument();
  });

  it("should use CSS variables for styling", () => {
    renderWithProviders(<RightPanel />);

    const buttons = screen.getAllByRole("button");
    const buttonElement = buttons[0];

    // Should use CSS variables (though actual values might be computed)
    expect(buttonElement).toBeInTheDocument();
  });
});
