import { render, screen, cleanup } from "@testing-library/react";
import { describe, it, expect, vi, afterEach } from "vitest";
import "./test-env";
import { userEvent } from "@testing-library/user-event";
import EmptyFeedState from "@/components/mobile/EmptyFeedState";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";

// Mock next/navigation
vi.mock("next/navigation", () => ({
  useRouter: () => ({
    push: vi.fn(),
    replace: vi.fn(),
    prefetch: vi.fn(),
  }),
}));

// Helper function to render with ChakraProvider
const renderWithProvider = (component: React.ReactElement) => {
  return render(
    <ChakraProvider value={defaultSystem}>{component}</ChakraProvider>,
  );
};

describe("EmptyFeedState", () => {
  afterEach(() => {
    cleanup();
  });

  it("should render empty state message", () => {
    renderWithProvider(<EmptyFeedState />);

    // Check for main heading
    expect(
      screen.getByRole("heading", { name: /no feeds yet/i }),
    ).toBeInTheDocument();

    // Check for descriptive text
    expect(
      screen.getByText(/start by adding your first rss feed/i),
    ).toBeInTheDocument();
  });

  it("should display icon", () => {
    renderWithProvider(<EmptyFeedState />);

    // Check that the icon container is present
    const iconContainer = screen.getByTestId("empty-state-icon");
    expect(iconContainer).toBeInTheDocument();
  });

  it("should render call-to-action button", () => {
    renderWithProvider(<EmptyFeedState />);

    // Check for CTA button
    const button = screen.getByRole("link", { name: /add your first feed/i });
    expect(button).toBeInTheDocument();
  });

  it("should have correct link to feed registration", () => {
    renderWithProvider(<EmptyFeedState />);

    const link = screen.getByRole("link", { name: /add your first feed/i });
    expect(link).toHaveAttribute("href", "/mobile/feeds/register");
  });

  it("should be accessible", () => {
    renderWithProvider(<EmptyFeedState />);

    // Check for proper ARIA labels
    const container = screen.getByRole("region");
    expect(container).toBeInTheDocument();
  });

  it("should handle button hover states", async () => {
    const user = userEvent.setup();
    renderWithProvider(<EmptyFeedState />);

    const button = screen.getByRole("link", { name: /add your first feed/i });

    // Button should be interactive
    await user.hover(button);
    expect(button).toBeInTheDocument();
  });
});
