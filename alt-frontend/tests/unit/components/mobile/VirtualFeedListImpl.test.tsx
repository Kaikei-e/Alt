import React from "react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor, act } from "@testing-library/react";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import { VirtualFeedListImpl } from '../../../src/VirtualFeedListImpl";
import { Feed } from "@/schema/feed";

// Mock components
vi.mock("./VirtualFeedListCore", () => ({
  VirtualFeedListCore: ({ feeds }: { feeds: Feed[] }) => (
    <div data-testid="virtual-feed-list-core">
      Fixed Sizing Mode - {feeds.length} feeds
    </div>
  ),
}));

vi.mock("./DynamicVirtualFeedList", () => ({
  DynamicVirtualFeedList: ({
    feeds,
    onMeasurementError,
  }: {
    feeds: Feed[];
    onMeasurementError: (error: Error) => void;
  }) => (
    <div data-testid="dynamic-virtual-feed-list">
      Dynamic Sizing Mode - {feeds.length} feeds
      <button onClick={() => onMeasurementError(new Error("Test error"))}>
        Trigger Error
      </button>
    </div>
  ),
}));

// Mock FeatureFlagManager
const mockGetFlags = vi.fn();
const mockUpdateFlags = vi.fn();
vi.mock("@/utils/featureFlags", () => ({
  FeatureFlagManager: {
    getInstance: vi.fn(() => ({
      getFlags: mockGetFlags,
      updateFlags: mockUpdateFlags,
    })),
  },
}));

// Mock useWindowSize
vi.mock("@/hooks/useWindowSize", () => ({
  useWindowSize: vi.fn(() => ({ width: 1024, height: 768 })),
}));

describe("VirtualFeedListImpl", () => {
  const createFeed = (
    id: string,
    title: string,
    description: string,
  ): Feed => ({
    id,
    title,
    description,
    link: `https://example.com/${id}`,
    published: new Date().toISOString(),
  });

  const shortFeeds = Array.from({ length: 50 }, (_, i) =>
    createFeed(`short-${i}`, `Short ${i}`, "Brief description"),
  );

  const longFeeds = Array.from(
    { length: 150 },
    (_, i) => createFeed(`long-${i}`, `Long Feed ${i}`, "A".repeat(600)), // Long description > 500 chars
  );

  const defaultProps = {
    feeds: shortFeeds,
    readFeeds: new Set<string>(),
    onMarkAsRead: vi.fn(),
  };

  beforeEach(() => {
    vi.clearAllMocks();
    mockGetFlags.mockReturnValue({
      enableDynamicSizing: "auto",
      enableVirtualization: true,
      forceVirtualization: false,
      debugMode: false,
      virtualizationThreshold: 200,
    });
  });

  const renderWithChakra = (ui: React.ReactElement) => {
    return render(<ChakraProvider value={defaultSystem}>{ui}</ChakraProvider>);
  };

  it("should use fixed sizing for small feed count", () => {
    renderWithChakra(<VirtualFeedListImpl {...defaultProps} />);

    expect(screen.getByTestId("virtual-feed-list-core")).toBeInTheDocument();
    expect(
      screen.queryByTestId("dynamic-virtual-feed-list"),
    ).not.toBeInTheDocument();
  });

  it("should use fixed sizing when dynamic sizing is disabled", () => {
    mockGetFlags.mockReturnValue({
      enableDynamicSizing: false,
      enableVirtualization: true,
      forceVirtualization: false,
      debugMode: false,
      virtualizationThreshold: 200,
    });

    renderWithChakra(
      <VirtualFeedListImpl {...defaultProps} feeds={longFeeds} />,
    );

    expect(screen.getByTestId("virtual-feed-list-core")).toBeInTheDocument();
    expect(
      screen.queryByTestId("dynamic-virtual-feed-list"),
    ).not.toBeInTheDocument();
  });

  it("should use dynamic sizing for large feed count with variable content", () => {
    renderWithChakra(
      <VirtualFeedListImpl {...defaultProps} feeds={longFeeds} />,
    );

    expect(screen.getByTestId("dynamic-virtual-feed-list")).toBeInTheDocument();
    expect(
      screen.queryByTestId("virtual-feed-list-core"),
    ).not.toBeInTheDocument();
  });

  it("should fallback to fixed sizing when dynamic sizing encounters error", async () => {
    renderWithChakra(
      <VirtualFeedListImpl {...defaultProps} feeds={longFeeds} />,
    );

    // Initially should use dynamic sizing
    expect(screen.getByTestId("dynamic-virtual-feed-list")).toBeInTheDocument();

    // Trigger error
    const errorButton = screen.getByText("Trigger Error");
    await act(async () => {
      errorButton.click();
    });

    // Should fallback to fixed sizing
    await waitFor(() => {
      expect(screen.getByTestId("virtual-feed-list-core")).toBeInTheDocument();
    });

    expect(
      screen.queryByTestId("dynamic-virtual-feed-list"),
    ).not.toBeInTheDocument();
    expect(mockUpdateFlags).toHaveBeenCalledWith({
      enableDynamicSizing: false,
    });
  });

  it("should not use dynamic sizing for feeds without variable content", () => {
    // Create feeds with uniform, short content
    const uniformFeeds = Array.from(
      { length: 150 },
      (_, i) => createFeed(`uniform-${i}`, `Feed ${i}`, "Short description"), // All have short descriptions
    );

    renderWithChakra(
      <VirtualFeedListImpl {...defaultProps} feeds={uniformFeeds} />,
    );

    // Should use fixed sizing because content is not variable
    expect(screen.getByTestId("virtual-feed-list-core")).toBeInTheDocument();
    expect(
      screen.queryByTestId("dynamic-virtual-feed-list"),
    ).not.toBeInTheDocument();
  });

  it("should handle empty feeds array", () => {
    renderWithChakra(<VirtualFeedListImpl {...defaultProps} feeds={[]} />);

    expect(screen.getByTestId("virtual-feed-list-core")).toBeInTheDocument();
    expect(screen.getByText("Fixed Sizing Mode - 0 feeds")).toBeInTheDocument();
  });

  it("should persist error state after dynamic sizing error", async () => {
    const { rerender } = renderWithChakra(
      <VirtualFeedListImpl {...defaultProps} feeds={longFeeds} />,
    );

    // Trigger error
    const errorButton = screen.getByText("Trigger Error");
    await act(async () => {
      errorButton.click();
    });

    // Wait for fallback
    await waitFor(() => {
      expect(screen.getByTestId("virtual-feed-list-core")).toBeInTheDocument();
    });

    // Rerender with new props - should still use fixed sizing
    rerender(
      <ChakraProvider value={defaultSystem}>
        <VirtualFeedListImpl
          {...defaultProps}
          feeds={[...longFeeds, createFeed("new", "New Feed", "A".repeat(600))]}
        />
      </ChakraProvider>,
    );

    expect(screen.getByTestId("virtual-feed-list-core")).toBeInTheDocument();
    expect(
      screen.queryByTestId("dynamic-virtual-feed-list"),
    ).not.toBeInTheDocument();
  });
});
