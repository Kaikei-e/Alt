import React from "react";
import { render, screen } from "@testing-library/react";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import { ReadingAnalytics } from "@/components/desktop/analytics/ReadingAnalytics";
import { mockAnalytics } from "@/data/mockAnalyticsData";
import { describe, it, expect } from "vitest";

const renderWithChakra = (ui: React.ReactElement) => {
  return render(<ChakraProvider value={defaultSystem}>{ui}</ChakraProvider>);
};

describe("ReadingAnalytics", () => {
  it("should display today stats correctly", () => {
    renderWithChakra(
      <ReadingAnalytics analytics={mockAnalytics} isLoading={false} />,
    );

    // Use more specific text matching to avoid duplicates
    expect(screen.getByText("12")).toBeInTheDocument(); // articles read
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

  it("should render articles text properly", () => {
    renderWithChakra(
      <ReadingAnalytics analytics={mockAnalytics} isLoading={false} />,
    );

    const articlesTexts = screen.getAllByText("Articles");
    expect(articlesTexts.length).toBeGreaterThan(0);
    expect(articlesTexts[0]).toBeInTheDocument();
  });

  it("should show loading state", () => {
    renderWithChakra(<ReadingAnalytics analytics={null} isLoading={true} />);

    // Chakra Spinner doesn't have progressbar role by default
    const spinner = document.querySelector(".chakra-spinner");
    expect(spinner).toBeInTheDocument();
  });

  it("should show no data message when analytics is null", () => {
    renderWithChakra(<ReadingAnalytics analytics={null} isLoading={false} />);

    const noDataMessages = screen.getAllByText(/No data available/);
    expect(noDataMessages.length).toBeGreaterThan(0);
  });
});
