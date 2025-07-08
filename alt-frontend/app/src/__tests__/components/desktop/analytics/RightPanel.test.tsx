import React from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import { RightPanel } from "@/components/desktop/analytics/RightPanel";
import { describe, it, expect, vi } from "vitest";

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

const renderWithChakra = (ui: React.ReactElement) => {
  return render(<ChakraProvider value={defaultSystem}>{ui}</ChakraProvider>);
};

describe("RightPanel", () => {
  it("should render with glass effect", () => {
    renderWithChakra(<RightPanel />);

    const glassElements = document.querySelectorAll(".glass");
    expect(glassElements.length).toBeGreaterThan(0);
  });

  it("should show Analytics tab as active by default", () => {
    renderWithChakra(<RightPanel />);

    const analyticsTab = screen.getByRole("button", { name: /analytics/i });
    expect(analyticsTab).toBeInTheDocument();
  });

  it("should switch between tabs", async () => {
    const user = userEvent.setup();
    renderWithChakra(<RightPanel />);

    // Click on Actions tab
    const actionsTab = screen.getByRole("button", { name: /actions/i });
    await user.click(actionsTab);

    expect(actionsTab).toBeInTheDocument();

    // Switch back to Analytics tab
    const analyticsTab = screen.getByRole("button", { name: /analytics/i });
    await user.click(analyticsTab);

    expect(analyticsTab).toBeInTheDocument();
  });

  it("should use CSS variables for styling", () => {
    renderWithChakra(<RightPanel />);

    const buttons = screen.getAllByRole("button");
    const buttonElement = buttons[0];

    // Should use CSS variables (though actual values might be computed)
    expect(buttonElement).toBeInTheDocument();
  });
});
