import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import { render, screen, waitFor } from "@testing-library/react";
import type React from "react";
import { describe, expect, it } from "vitest";
import { TrendingTopics } from "@/components/desktop/analytics/TrendingTopics";
import { mockTrendingTopics } from "@/data/mockAnalyticsData";

const renderWithChakra = (ui: React.ReactElement) => {
  return render(<ChakraProvider value={defaultSystem}>{ui}</ChakraProvider>);
};

describe("TrendingTopics", () => {
  it("should display trending topics correctly", async () => {
    renderWithChakra(<TrendingTopics topics={mockTrendingTopics} isLoading={false} />);

    await waitFor(() => {
      expect(screen.getByText("#AI")).toBeInTheDocument();
    });
    expect(screen.getByText("#React")).toBeInTheDocument();
    expect(screen.getByText("45 articles")).toBeInTheDocument();
  });

  it("should show glass effect styling", () => {
    renderWithChakra(<TrendingTopics topics={mockTrendingTopics} isLoading={false} />);

    const glassElements = document.querySelectorAll(".glass");
    expect(glassElements.length).toBeGreaterThan(0);
  });

  it("should display trend indicators correctly", async () => {
    const { container } = renderWithChakra(
      <TrendingTopics topics={mockTrendingTopics} isLoading={false} />
    );

    // テキストが複数の要素に分割されている可能性があるため、container全体のテキストを確認
    await waitFor(
      () => {
        // "+23%"は "+", "23", "%" として分割されている可能性がある
        // 親要素全体のテキストコンテンツを確認
        expect(container.textContent).toContain("+23%");
        expect(container.textContent).toContain("+12%");
      },
      { timeout: 3000 }
    );
  });

  it("should show loading state", () => {
    renderWithChakra(<TrendingTopics topics={[]} isLoading={true} />);

    // Chakra Spinner doesn't have progressbar role by default
    const spinner = document.querySelector(".chakra-spinner");
    expect(spinner).toBeInTheDocument();
  });

  it("should limit displayed topics to 6", async () => {
    const manyTopics = Array.from({ length: 10 }, (_, i) => ({
      ...mockTrendingTopics[0],
      tag: `Topic${i}`,
    }));

    renderWithChakra(<TrendingTopics topics={manyTopics} isLoading={false} />);

    // Wait for component to render, then check that only first 6 topics are shown
    await waitFor(
      () => {
        expect(screen.getByText("#Topic0")).toBeInTheDocument();
      },
      { timeout: 5000 }
    );

    // Verify first 6 topics are displayed
    expect(screen.getByText("#Topic1")).toBeInTheDocument();
    expect(screen.getByText("#Topic2")).toBeInTheDocument();
    expect(screen.getByText("#Topic3")).toBeInTheDocument();
    expect(screen.getByText("#Topic4")).toBeInTheDocument();
    expect(screen.getByText("#Topic5")).toBeInTheDocument();

    // Verify 7th topic is not displayed
    expect(screen.queryByText("#Topic6")).not.toBeInTheDocument();
    expect(screen.queryByText("#Topic7")).not.toBeInTheDocument();
    expect(screen.queryByText("#Topic8")).not.toBeInTheDocument();
    expect(screen.queryByText("#Topic9")).not.toBeInTheDocument();
  });
});
