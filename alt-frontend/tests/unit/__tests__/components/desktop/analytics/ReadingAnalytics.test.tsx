import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import { render, screen, waitFor } from "@testing-library/react";
import type React from "react";
import { describe, expect, it } from "vitest";
import { ReadingAnalytics } from "@/components/desktop/analytics/ReadingAnalytics";
import { mockAnalytics } from "@/data/mockAnalyticsData";

const renderWithChakra = (ui: React.ReactElement) => {
  return render(<ChakraProvider value={defaultSystem}>{ui}</ChakraProvider>);
};

describe("ReadingAnalytics", () => {
  it("should display today stats correctly", async () => {
    renderWithChakra(
      <ReadingAnalytics analytics={mockAnalytics} isLoading={false} />,
    );

    // Wait for component to render and use more specific text matching
    await waitFor(() => {
      expect(screen.getByText("12")).toBeInTheDocument(); // articles read
    });
    expect(screen.getByText(/45m/)).toBeInTheDocument(); // time spent
    expect(screen.getByText("Favorites")).toBeInTheDocument(); // favorites label
  });

  it("should show glass effect styling", () => {
    renderWithChakra(
      <ReadingAnalytics analytics={mockAnalytics} isLoading={false} />,
    );

    const glassElements = document.querySelectorAll(".glass");
    expect(glassElements.length).toBeGreaterThan(0);
  });

  it("should render articles text properly", async () => {
    renderWithChakra(
      <ReadingAnalytics analytics={mockAnalytics} isLoading={false} />,
    );

    await waitFor(() => {
      const articlesTexts = screen.getAllByText("Articles");
      expect(articlesTexts.length).toBeGreaterThan(0);
      expect(articlesTexts[0]).toBeInTheDocument();
    });
  });

  it("should show loading state", () => {
    renderWithChakra(<ReadingAnalytics analytics={null} isLoading={true} />);

    // Chakra Spinner doesn't have progressbar role by default
    const spinner = document.querySelector(".chakra-spinner");
    expect(spinner).toBeInTheDocument();
  });

  it("should show no data message when analytics is null", async () => {
    renderWithChakra(<ReadingAnalytics analytics={null} isLoading={false} />);

    await waitFor(() => {
      const noDataMessages = screen.getAllByText(/No data available/i);
      expect(noDataMessages.length).toBeGreaterThan(0);
    });
  });
});
