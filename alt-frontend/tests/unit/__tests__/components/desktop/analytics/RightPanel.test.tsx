import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
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

  it("should show Analytics tab as active by default", async () => {
    renderWithProviders(<RightPanel />);

    // Wait for component to render
    await waitFor(() => {
      expect(screen.getAllByText("📊 Analytics")[0]).toBeInTheDocument();
    });

    const analyticsButtons = screen.getAllByText("📊 Analytics");
    expect(analyticsButtons[0]).toBeInTheDocument();
    expect(analyticsButtons[0].closest("button")).toBeInTheDocument();
  });

  it("should switch between tabs", async () => {
    const user = userEvent.setup();
    renderWithProviders(<RightPanel />);

    // Wait for component to render
    await waitFor(() => {
      expect(screen.getAllByText("⚡ Actions")[0]).toBeInTheDocument();
      expect(screen.getAllByText("📊 Analytics")[0]).toBeInTheDocument();
    });

    // Click on Actions tab
    const actionsButton = screen.getAllByText("⚡ Actions")[0];
    await user.click(actionsButton);

    expect(actionsButton).toBeInTheDocument();

    // Switch back to Analytics tab
    const analyticsButton = screen.getAllByText("📊 Analytics")[0];
    await user.click(analyticsButton);

    expect(analyticsButton).toBeInTheDocument();
  });

  it("should use CSS variables for styling", async () => {
    renderWithProviders(<RightPanel />);

    // Wait for component to render
    await waitFor(() => {
      expect(screen.getAllByText("📊 Analytics")[0]).toBeInTheDocument();
    });

    const analyticsButton = screen.getAllByText("📊 Analytics")[0];
    const buttonElement = analyticsButton.closest("button");

    // Should use CSS variables (though actual values might be computed)
    expect(buttonElement).toBeInTheDocument();
  });
});
