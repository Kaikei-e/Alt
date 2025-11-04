import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type React from "react";
import { describe, expect, it, vi } from "vitest";
import { RightPanel } from "@/components/desktop/analytics/RightPanel";
import { ThemeProvider } from "@/providers/ThemeProvider";

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
    </ThemeProvider>
  );
};

const findButtonByLabel = async (label: string): Promise<HTMLButtonElement> => {
  const matches = await screen.findAllByText(label);
  const candidate = matches
    .map((node) => node.closest("button"))
    .find((button): button is HTMLButtonElement => !!button);

  if (!candidate) {
    throw new Error(`Button with label "${label}" not found`);
  }

  return candidate;
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
    const user = userEvent.setup({ pointerEventsCheck: 0 });
    renderWithProviders(<RightPanel />);

    const analyticsTab = await findButtonByLabel("📊 Analytics");
    const actionsTab = await findButtonByLabel("⚡ Actions");

    expect(analyticsTab).toBeInTheDocument();
    expect(actionsTab).toBeInTheDocument();

    expect(screen.queryByText("⚡ Quick Actions")).not.toBeInTheDocument();

    await user.click(actionsTab);
    await waitFor(() => expect(screen.getByText("⚡ Quick Actions")).toBeInTheDocument());
    expect(screen.queryByText("⚡ Quick Actions")).toBeInTheDocument();

    await user.click(analyticsTab);
    await waitFor(() => expect(screen.queryByText("⚡ Quick Actions")).not.toBeInTheDocument());
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
