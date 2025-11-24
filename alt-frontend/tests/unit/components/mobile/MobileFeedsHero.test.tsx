import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import "./test-env";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import { MobileFeedsHero } from "@/app/mobile/feeds/_components/MobileFeedsHero";

// Helper function to render with ChakraProvider
const renderWithProvider = (component: React.ReactElement) => {
  return render(
    <ChakraProvider value={defaultSystem}>{component}</ChakraProvider>,
  );
};

describe("MobileFeedsHero", () => {
  afterEach(() => {
    cleanup();
  });

  it("should render hero section with heading", () => {
    renderWithProvider(<MobileFeedsHero />);

    const heading = screen.getByRole("heading", { name: /your feeds/i });
    expect(heading).toBeInTheDocument();
  });

  it("should render HeroTip with LCP attributes", () => {
    renderWithProvider(<MobileFeedsHero />);

    const tipSection = screen.getByLabelText("Tip");
    expect(tipSection).toBeInTheDocument();
    expect(tipSection).toHaveAttribute("data-lcp-hero", "tip");
    expect(tipSection).toHaveClass("lcp-hero-tip");
  });

  it("should render tip text content", () => {
    renderWithProvider(<MobileFeedsHero />);

    const tipText = screen.getByText(/tip: you can swipe through your feeds/i);
    expect(tipText).toBeInTheDocument();
  });

  it("should use semantic HTML structure", () => {
    renderWithProvider(<MobileFeedsHero />);

    const header = screen.getByRole("banner");
    expect(header).toBeInTheDocument();

    const tipSection = screen.getByLabelText("Tip");
    expect(tipSection.tagName).toBe("SECTION");
  });

  it("should have proper accessibility attributes", () => {
    renderWithProvider(<MobileFeedsHero />);

    const tipSection = screen.getByLabelText("Tip");
    expect(tipSection).toHaveAttribute("aria-label", "Tip");
  });
});

